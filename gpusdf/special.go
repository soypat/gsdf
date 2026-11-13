package gpusdf

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"unsafe"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms2"
	"github.com/soypat/gsdf/glbuild"
)

// KernelConfig configures the built-in GPU evaluators of this package:
// [Batcher], [Polygon], [Lines2D] and [DisplaceMulti2D].
type KernelConfig struct {
	// WorkgroupSize is the size of the compute workgroup. If zero the device maximum (capped to 256) is used.
	WorkgroupSize int
	// MaxBatch is the maximum number of elements evaluated in a single GPU dispatch. If zero a default value is used.
	MaxBatch int
}

func (kc KernelConfig) resolve(dev *Device) (wg, maxBatch int, err error) {
	if dev == nil || dev.MaxWorkgroupSize() == 0 {
		return 0, 0, errDeviceNotReady
	} else if kc.WorkgroupSize < 0 {
		return 0, 0, errZeroWorkgroupSize
	} else if kc.MaxBatch < 0 {
		return 0, 0, errZeroMaxBatch
	}
	wg = kc.WorkgroupSize
	if wg == 0 {
		wg = min(256, dev.MaxWorkgroupSize())
	} else if wg > dev.MaxWorkgroupSize() {
		return 0, 0, errWorkgroupSizeLimit
	}
	maxBatch = kc.MaxBatch
	if maxBatch == 0 {
		maxBatch = defaultMaxBatch
	}
	return wg, maxBatch, nil
}

// evaluate2 evaluates 2D positions on k splitting the work in batches.
func evaluate2(k *kernel, pos []ms2.Vec, dist []float32) error {
	if len(pos) != len(dist) {
		return errMismatchLength
	} else if len(pos) == 0 {
		return errEmptyBuffers
	}
	for len(pos) > 0 {
		n := min(len(pos), k.maxBatch)
		err := k.submit(sliceBytes(pos[:n]), nil, n)
		if err != nil {
			return err
		}
		err = k.read(sliceBytes(dist[:n]))
		if err != nil {
			return err
		}
		pos = pos[n:]
		dist = dist[n:]
	}
	return nil
}

// Batcher executes element-wise binary operations on float32 buffers on the GPU.
type Batcher struct {
	k       kernel
	dev     *Device
	wg      int
	batch   int
	lastOp  []byte
	scratch bytes.Buffer
}

// Configure releases previous GPU resources and prepares the Batcher for execution on dev.
func (b *Batcher) Configure(dev *Device, cfg KernelConfig) error {
	b.Release()
	wg, batch, err := cfg.resolve(dev)
	if err != nil {
		return err
	}
	b.dev = dev
	b.wg = wg
	b.batch = batch
	return nil
}

// Release releases all GPU resources held by b.
func (b *Batcher) Release() {
	b.k.release()
	b.dev = nil
	b.lastOp = b.lastOp[:0]
	b.scratch.Reset()
}

// ExecuteRawBinaryOperation executes the WGSL function body opSource for each element of A and B and stores the result in dst.
// The body has access to the f32 arguments a and b and must return a f32 value, i.e: "return min(a,b);".
// The compute program is recompiled only when opSource changes between calls.
func (b *Batcher) ExecuteRawBinaryOperation(opSource []byte, dst, A, B []float32) error {
	if b.dev == nil {
		return errNotConfigured
	} else if len(dst) != len(A) || len(A) != len(B) {
		return errMismatchLength
	} else if len(dst) == 0 {
		return errEmptyBuffers
	}
	if !bytes.Equal(opSource, b.lastOp) || b.k.maxBatch == 0 {
		b.scratch.Reset()
		fmt.Fprintf(&b.scratch, binopShader, opSource, b.wg, b.wg)
		err := b.k.configure(b.dev, kernelConfig{
			source:      b.scratch.Bytes(),
			wg:          b.wg,
			maxBatch:    b.batch,
			inputStride: [2]int{4, 4},
		})
		if err != nil {
			b.lastOp = b.lastOp[:0]
			return err
		}
		b.lastOp = append(b.lastOp[:0], opSource...)
	}
	for len(dst) > 0 {
		n := min(len(dst), b.k.maxBatch)
		err := b.k.submit(sliceBytes(A[:n]), sliceBytes(B[:n]), n)
		if err != nil {
			return err
		}
		err = b.k.read(sliceBytes(dst[:n]))
		if err != nil {
			return err
		}
		dst, A, B = dst[n:], A[n:], B[n:]
	}
	return nil
}

const binopShader = `
@group(0) @binding(0) var<storage, read> gsdfA: array<f32>;
@group(0) @binding(1) var<storage, read> gsdfB: array<f32>;
@group(0) @binding(2) var<storage, read_write> gsdfOut: array<f32>;
@group(0) @binding(3) var<uniform> gsdfParams: vec4u;

fn op(a: f32, b: f32) -> f32 {
%s
}

@compute @workgroup_size(%d, 1, 1)
fn main(@builtin(global_invocation_id) gid: vec3u, @builtin(num_workgroups) nwg: vec3u) {
	let idx = gid.x + gid.y*nwg.x*%du;
	if (idx >= gsdfParams.x) {
		return;
	}
	gsdfOut[idx] = op(gsdfA[idx], gsdfB[idx]);
}
`

// I/O bindings shared by built-in 2D evaluators.
const builtinIO2D = `
@group(0) @binding(0) var<storage, read> gsdfPos: array<vec2f>;
@group(0) @binding(1) var<storage, read_write> gsdfDist: array<f32>;
@group(0) @binding(2) var<uniform> gsdfParams: vec4u; // x: number of positions, y: f32 bits of parameter.
`

// Polygon implements a direct polygon SDF evaluation on the GPU.
type Polygon struct {
	// Vertices of the polygon. Vertices are uploaded to the GPU during Configure.
	Vertices []ms2.Vec
	k        kernel
	evals    uint64
}

// Configure uploads p.Vertices to the GPU and compiles the polygon program. Must be called again if Vertices are modified.
func (p *Polygon) Configure(dev *Device, cfg KernelConfig) error {
	p.k.release()
	if len(p.Vertices) < 3 {
		return errors.New("gpusdf: polygon requires at least 3 vertices")
	}
	wg, batch, err := cfg.resolve(dev)
	if err != nil {
		return err
	}
	obj, err := glbuild.MakeShaderBufferReadOnly([]byte("gsdfVerts"), p.Vertices)
	if err != nil {
		return err
	}
	obj.Binding = 0
	src := fmt.Appendf(nil, polygonShader, builtinIO2D, wg, wg)
	return p.k.configure(dev, kernelConfig{
		source:      src,
		wg:          wg,
		maxBatch:    batch,
		inputStride: [2]int{int(unsafe.Sizeof(ms2.Vec{}))},
		objs:        []glbuild.ShaderObject{obj},
	})
}

// Release releases all GPU resources held by p.
func (p *Polygon) Release() { p.k.release() }

// Evaluate implements gleval.SDF2.
func (p *Polygon) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	err := evaluate2(&p.k, pos, dist)
	if err == nil {
		p.evals += uint64(len(pos))
	}
	return err
}

// Evaluations returns total evaluations performed succesfully during p's lifetime.
func (p *Polygon) Evaluations() uint64 { return p.evals }

// Bounds implements gleval.SDF2.
func (p *Polygon) Bounds() ms2.Box {
	bb := ms2.Box{
		Min: ms2.Vec{X: math32.Inf(1), Y: math32.Inf(1)},
		Max: ms2.Vec{X: math32.Inf(-1), Y: math32.Inf(-1)},
	}
	for i := range p.Vertices {
		bb.Max = ms2.MaxElem(bb.Max, p.Vertices[i])
		bb.Min = ms2.MinElem(bb.Min, p.Vertices[i])
	}
	return bb
}

// winding number from http://geomalgorithms.com/a03-_inclusion.html
const polygonShader = `%s
@group(1) @binding(0) var<storage, read> gsdfVerts: array<vec2f>;

fn poly(p: vec2f) -> f32 {
	let num = i32(arrayLength(&gsdfVerts));
	var d = dot(p-gsdfVerts[0], p-gsdfVerts[0]);
	var s = 1.0;
	var j = num-1;
	for (var i=0; i<num; i++) {
		let vi = gsdfVerts[i];
		let vj = gsdfVerts[j];
		let e = vj - vi;
		let w = p - vi;
		let b = w - e*clamp(dot(w,e)/dot(e,e), 0.0, 1.0);
		d = min(d, dot(b,b));
		let cond = vec3<bool>((p.y >= vi.y), (p.y < vj.y), (e.x*w.y > e.y*w.x));
		if (all(cond) || all(!cond)) {
			s = -s;
		}
		j = i;
	}
	return s*sqrt(d);
}

@compute @workgroup_size(%d, 1, 1)
fn main(@builtin(global_invocation_id) gid: vec3u, @builtin(num_workgroups) nwg: vec3u) {
	let idx = gid.x + gid.y*nwg.x*%du;
	if (idx >= gsdfParams.x) {
		return;
	}
	gsdfDist[idx] = poly(gsdfPos[idx]);
}
`

// Lines2D implements a direct SDF evaluation of a set of 2D line segments of equal width on the GPU.
type Lines2D struct {
	// Lines are the segments. Lines are uploaded to the GPU during Configure.
	Lines [][2]ms2.Vec
	// Width of the lines. Can be modified between evaluations.
	Width float32
	k     kernel
	evals uint64
}

// Configure uploads l.Lines to the GPU and compiles the program. Must be called again if Lines are modified.
func (l *Lines2D) Configure(dev *Device, cfg KernelConfig) error {
	l.k.release()
	if len(l.Lines) == 0 {
		return errors.New("gpusdf: no lines to configure")
	}
	wg, batch, err := cfg.resolve(dev)
	if err != nil {
		return err
	}
	obj, err := glbuild.MakeShaderBufferReadOnly([]byte("gsdfLines"), l.Lines)
	if err != nil {
		return err
	}
	obj.Binding = 0
	src := fmt.Appendf(nil, linesShader, builtinIO2D, wg, wg)
	return l.k.configure(dev, kernelConfig{
		source:      src,
		wg:          wg,
		maxBatch:    batch,
		inputStride: [2]int{int(unsafe.Sizeof(ms2.Vec{}))},
		objs:        []glbuild.ShaderObject{obj},
	})
}

// Release releases all GPU resources held by l.
func (l *Lines2D) Release() { l.k.release() }

// Evaluate implements gleval.SDF2.
func (l *Lines2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	l.k.setParam(math.Float32bits(l.Width / 2))
	err := evaluate2(&l.k, pos, dist)
	if err == nil {
		l.evals += uint64(len(pos))
	}
	return err
}

// Evaluations returns total evaluations performed succesfully during l's lifetime.
func (l *Lines2D) Evaluations() uint64 { return l.evals }

// Bounds implements gleval.SDF2.
func (l *Lines2D) Bounds() ms2.Box {
	bb := ms2.Box{
		Min: ms2.Vec{X: math32.Inf(1), Y: math32.Inf(1)},
		Max: ms2.Vec{X: math32.Inf(-1), Y: math32.Inf(-1)},
	}
	off := l.Width / 2
	offp := ms2.Vec{X: off, Y: off}
	offm := ms2.Vec{X: -off, Y: -off}
	for _, v1v2 := range l.Lines {
		bb.Max = ms2.MaxElem(bb.Max, ms2.Add(v1v2[0], offp))
		bb.Min = ms2.MinElem(bb.Min, ms2.Add(v1v2[0], offm))
		bb.Max = ms2.MaxElem(bb.Max, ms2.Add(v1v2[1], offp))
		bb.Min = ms2.MinElem(bb.Min, ms2.Add(v1v2[1], offm))
	}
	return bb
}

const linesShader = `%s
@group(1) @binding(0) var<storage, read> gsdfLines: array<vec4f>;

fn segment2(p: vec2f, a: vec2f, b: vec2f) -> f32 {
	let pa = p-a;
	let ba = b-a;
	let h = clamp(dot(pa,ba)/dot(ba,ba), 0.0, 1.0);
	let dv = pa - ba*h;
	return dot(dv, dv);
}

fn lines(p: vec2f) -> f32 {
	let num = i32(arrayLength(&gsdfLines));
	var d2 = 1.0e23;
	for (var i=0; i<num; i++) {
		let v1v2 = gsdfLines[i];
		d2 = min(d2, segment2(p, v1v2.xy, v1v2.zw));
	}
	return sqrt(d2) - bitcast<f32>(gsdfParams.y);
}

@compute @workgroup_size(%d, 1, 1)
fn main(@builtin(global_invocation_id) gid: vec3u, @builtin(num_workgroups) nwg: vec3u) {
	let idx = gid.x + gid.y*nwg.x*%du;
	if (idx >= gsdfParams.x) {
		return;
	}
	gsdfDist[idx] = lines(gsdfPos[idx]);
}
`

// DisplaceMulti2D evaluates the union of copies of a 2D shape element displaced by each of Displacements on the GPU.
type DisplaceMulti2D struct {
	// Displacements of the element. Uploaded to the GPU during Configure.
	Displacements []ms2.Vec
	elemBB        ms2.Box
	k             kernel
	evals         uint64
}

// Configure generates the program for element with programmer and uploads Displacements to the GPU.
// Must be called again if Displacements are modified.
func (disp *DisplaceMulti2D) Configure(dev *Device, programmer *glbuild.Programmer, element glbuild.Shader2D, cfg KernelConfig) error {
	disp.k.release()
	if len(disp.Displacements) == 0 {
		return errors.New("gpusdf: no displacements to configure")
	}
	wg, batch, err := cfg.resolve(dev)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	basename, _, objs, err := programmer.WriteSDFDecl(&buf, element)
	if err != nil {
		return err
	}
	// Displacements are bound after element's own shader objects.
	binding := 0
	for i := range objs {
		if objs[i].IsBindable() {
			binding = max(binding, objs[i].Binding+1)
		}
	}
	obj, err := glbuild.MakeShaderBufferReadOnly([]byte("gsdfDisplace"), disp.Displacements)
	if err != nil {
		return err
	}
	obj.Binding = binding
	objs = append(objs, obj)
	buf.WriteString(builtinIO2D)
	fmt.Fprintf(&buf, multiDisplaceShader, glbuild.ObjectsBindGroup, binding, basename, wg, wg)
	err = disp.k.configure(dev, kernelConfig{
		source:      buf.Bytes(),
		wg:          wg,
		maxBatch:    batch,
		inputStride: [2]int{int(unsafe.Sizeof(ms2.Vec{}))},
		objs:        objs,
	})
	if err != nil {
		return err
	}
	disp.elemBB = element.Bounds()
	return nil
}

// Release releases all GPU resources held by disp.
func (disp *DisplaceMulti2D) Release() { disp.k.release() }

// Evaluate implements gleval.SDF2.
func (disp *DisplaceMulti2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	err := evaluate2(&disp.k, pos, dist)
	if err == nil {
		disp.evals += uint64(len(pos))
	}
	return err
}

// Evaluations returns total evaluations performed succesfully during disp's lifetime.
func (disp *DisplaceMulti2D) Evaluations() uint64 { return disp.evals }

// Bounds implements gleval.SDF2.
func (disp *DisplaceMulti2D) Bounds() ms2.Box {
	var bb ms2.Box
	elemBox := disp.elemBB
	for i := range disp.Displacements {
		bb = bb.Union(elemBox.Add(disp.Displacements[i]))
	}
	return bb
}

const multiDisplaceShader = `
@group(%d) @binding(%d) var<storage, read> gsdfDisplace: array<vec2f>;

fn multidisplace(p: vec2f) -> f32 {
	let num = i32(arrayLength(&gsdfDisplace));
	var d = 1.0e23;
	for (var i=0; i<num; i++) {
		d = min(d, %s(p - gsdfDisplace[i]));
	}
	return d;
}

@compute @workgroup_size(%d, 1, 1)
fn main(@builtin(global_invocation_id) gid: vec3u, @builtin(num_workgroups) nwg: vec3u) {
	let idx = gid.x + gid.y*nwg.x*%du;
	if (idx >= gsdfParams.x) {
		return;
	}
	gsdfDist[idx] = multidisplace(gsdfPos[idx]);
}
`
