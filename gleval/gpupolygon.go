//go:build !tinygo && cgo

package gleval

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/chewxy/math32"
	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
)

// PolygonGPU implements a direct polygon evaluation via GPU.
type PolygonGPU struct {
	Vertices    []ms2.Vec
	evaluations uint64
	shader      string
	invocX      int
}

func (poly *PolygonGPU) Bounds() ms2.Box {
	bb := ms2.Box{
		Min: ms2.Vec{X: math32.Inf(1), Y: math32.Inf(1)},
		Max: ms2.Vec{X: math32.Inf(-1), Y: math32.Inf(-1)},
	}
	for i := range poly.Vertices {
		bb.Max = ms2.MaxElem(bb.Max, poly.Vertices[i])
		bb.Min = ms2.MinElem(bb.Min, poly.Vertices[i])
	}
	return bb
}

func (poly *PolygonGPU) Configure(cfg ComputeConfig) error {
	if cfg.InvocX < 1 {
		return errZeroInvoc
	}
	poly.invocX = cfg.InvocX
	poly.shader = fmt.Sprintf(polyshader, cfg.InvocX)
	return nil
}

func (poly *PolygonGPU) Evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	} else if poly.shader == "" {
		return errors.New("need to initialize PolygonGPU before first use")
	}
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: poly.shader})
	if err != nil {
		return fmt.Errorf("compiling GL program: %w", err)
	}
	defer prog.Delete()
	prog.Bind()
	err = glgl.Err()
	if err != nil {
		return fmt.Errorf("binding PolygonGPU program: %w", err)
	}
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadvbo(poly.Vertices, 2, gl.STATIC_DRAW)
	err = glgl.Err()
	if err != nil {
		return err
	}
	p.Pin(&ssbo)
	defer p.Unpin()
	defer gl.DeleteBuffers(1, &ssbo)

	err = computeEvaluate(pos, dist, poly.invocX)
	if err != nil {
		return err
	}
	poly.evaluations += uint64(len(dist))
	return nil
}

func computeEvaluate[T ms2.Vec | ms3.Quat](pos []T, dist []float32, invocX int) (err error) {
	if len(pos) != len(dist) {
		return errors.New("positional and distance buffers not equal in length")
	} else if len(dist) == 0 {
		return errors.New("zero length buffers")
	} else if invocX < 1 {
		return errors.New("zero or negative invocation size")
	}
	var p runtime.Pinner
	var posSSBO, distSSBO uint32
	p.Pin(&posSSBO)
	p.Pin(&distSSBO)
	defer p.Unpin()

	posSSBO = loadvbo(pos, 0, gl.STATIC_DRAW)
	err = glgl.Err()
	if err != nil {
		return err
	}
	defer gl.DeleteBuffers(1, &posSSBO)

	distSSBO = createvbo(elemSize[float32]()*len(dist), 1, gl.DYNAMIC_READ)
	err = glgl.Err()
	if err != nil {
		return err
	}
	nWorkX := (len(dist) + invocX - 1) / invocX
	defer gl.DeleteBuffers(1, &distSSBO)
	gl.DispatchCompute(uint32(nWorkX), 1, 1)
	err = glgl.Err()
	if err != nil {
		return err
	}
	gl.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT)
	err = glgl.Err()
	if err != nil {
		return err
	}
	err = getvbo(distSSBO, dist)
	if err != nil {
		return err
	}
	return nil
}

func loadvbo[T elem](slice []T, base, usage uint32) (ssbo uint32) {
	p := runtime.Pinner{}
	p.Pin(&ssbo)
	gl.GenBuffers(1, &ssbo)
	p.Unpin()
	gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, ssbo)
	size := len(slice) * elemSize[T]()
	gl.BufferData(gl.SHADER_STORAGE_BUFFER, size, unsafe.Pointer(&slice[0]), usage)
	gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, base, ssbo)
	return ssbo
}

func createvbo(size int, base, usage uint32) (ssbo uint32) {
	gl.GenBuffers(1, &ssbo)
	gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, ssbo)
	gl.BufferData(gl.SHADER_STORAGE_BUFFER, size, nil, usage)
	gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, base, ssbo)
	return ssbo
}

func getvbo[T elem](ssbo uint32, buf []T) error {
	singleSize := elemSize[T]()
	bufSize := singleSize * len(buf)
	gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, ssbo)
	ptr := gl.MapBufferRange(gl.SHADER_STORAGE_BUFFER, 0, bufSize, gl.MAP_READ_BIT)
	if ptr == nil {
		return errors.New("failed to map buffer")
	}
	defer gl.UnmapBuffer(gl.SHADER_STORAGE_BUFFER)
	gpuBytes := unsafe.Slice((*byte)(ptr), bufSize)
	bufBytes := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), bufSize)
	copy(bufBytes, gpuBytes)
	return glgl.Err()
}

// winding number from http://geomalgorithms.com/a03-_inclusion.html
const polyshader = `#version 430

layout(local_size_x = %d, local_size_y = 1, local_size_z = 1) in;

// Input: 2D positions at which to evaluate SDF.
layout(std430, binding = 0) buffer PositionsBuffer {
    vec2 vbo_positions[];
};

// Output: Result of SDF evaluation are the distances. Maps to position buffer.
layout(std430, binding = 1) buffer DistancesBuffer {
    float vbo_distances[];
};

layout(std430, binding = 2) buffer VerticesBuffer {
    vec2 v[];
};

float poly(vec2 p){
	const int num = v.length();
	float d = dot(p-v[0],p-v[0]);
	float s = 1.0;
	for( int i=0, j=num-1; i<num; j=i, i++ )
	{
		vec2 vi = v[i];
		vec2 vj = v[j];
		vec2 e = vj - vi;
		vec2 w = p - vi;
		vec2 b = w - e*clamp( dot(w,e)/dot(e,e), 0.0, 1.0 );
		d = min( d, dot(b,b) );
		bvec3 cond = bvec3( p.y>=vi.y,
							p.y <vj.y,
							e.x*w.y>e.y*w.x );
		if( all(cond) || all(not(cond)) ) s=-s;
	}
	return s*sqrt(d);
}

void main() {
	int idx = int( gl_GlobalInvocationID.x );

	vec2 p = vbo_positions[idx];    // Get position to evaluate SDF at.
	vbo_distances[idx] = poly(p);   // Evaluate SDF and store to distance buffer.
}
` + "\x00"

type elem interface {
	float32 | ms2.Vec | ms3.Vec | ms3.Quat
}

func elemSize[T elem]() int {
	var z T
	return int(unsafe.Sizeof(z))
}
