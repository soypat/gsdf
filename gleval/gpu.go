package gleval

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"unsafe"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf/glbuild"
)

var errZeroInvoc = errors.New("zero or negative workgroup invocation size, ComputeConfig must have non-zero InvocX field")

// Init1x1GLFW starts a 1x1 sized GLFW so that user can start working with GPU.
// It returns a termination function that should be called when user is done running loads on GPU.
func Init1x1GLFW() (terminate func(), err error) {
	_, terminate, err = glgl.InitWithCurrentWindow33(glgl.WindowConfig{
		Title:         "compute",
		Version:       [2]int{4, 6},
		OpenGLProfile: glgl.ProfileCompat,
		Width:         1,
		Height:        1,
	})
	return terminate, err
}

// NewComputeGPUSDF3 instantiates a [SDF3] that runs on the GPU.
func NewComputeGPUSDF3(glglSourceCode io.Reader, bb ms3.Box, cfg ComputeConfig) (*SDF3Compute, error) {
	if cfg.InvocX < 1 {
		return nil, errZeroInvoc
	}
	combinedSource, err := glgl.ParseCombined(glglSourceCode)
	if err != nil {
		return nil, err
	}
	glprog, err := glgl.CompileProgram(combinedSource)
	if err != nil {
		return nil, errors.New(string(combinedSource.Compute) + "\n" + err.Error())
	}
	sdf := SDF3Compute{
		prog:   glprog,
		bb:     bb,
		invocX: cfg.InvocX,
	}
	return &sdf, nil
}

type SDF3Compute struct {
	prog           glgl.Program
	bb             ms3.Box
	evals          uint64
	alignAuxiliary []ms3.Quat
	invocX         int
}

type ComputeConfig struct {
	// InvocX represents the size of the worker group in warps/invocations as configured in the shader.
	// This is configured in a declaration of the following style in the shader:
	//  layout(local_size_x = <InvocX>, local_size_y = 1, local_size_z = 1) in;
	InvocX int
}

func (sdf *SDF3Compute) Bounds() ms3.Box {
	return sdf.bb
}

// Evaluations returns total evaluations performed succesfully during sdf's lifetime.
func (sdf *SDF3Compute) Evaluations() uint64 { return sdf.evals }

func (sdf *SDF3Compute) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf.prog.Bind()
	defer sdf.prog.Unbind()
	err := glgl.Err()
	if err != nil {
		return fmt.Errorf("binding SDF3Compute program: %w", err)
	}
	if len(sdf.alignAuxiliary) < len(pos) {
		sdf.alignAuxiliary = append(sdf.alignAuxiliary, make([]ms3.Quat, len(pos)-len(sdf.alignAuxiliary))...)
	}
	aligned := sdf.alignAuxiliary[:len(pos)]
	for i := range aligned {
		aligned[i].V = pos[i]
	}
	err = computeEvaluate(aligned, dist, sdf.invocX)
	if err != nil {
		return err
	}
	sdf.evals += uint64(len(pos))
	return nil
}

// NewComputeGPUSDF2 instantiates a [SDF2] that runs on the GPU.
func NewComputeGPUSDF2(glglSourceCode io.Reader, bb ms2.Box, cfg ComputeConfig) (*SDF2Compute, error) {
	if cfg.InvocX < 1 {
		return nil, errZeroInvoc
	}
	combinedSource, err := glgl.ParseCombined(glglSourceCode)
	if err != nil {
		return nil, err
	}
	glprog, err := glgl.CompileProgram(combinedSource)
	if err != nil {
		return nil, errors.New(string(combinedSource.Compute) + "\n" + err.Error())
	}
	sdf := SDF2Compute{
		prog:   glprog,
		bb:     bb,
		invocX: cfg.InvocX,
	}
	return &sdf, nil
}

type SDF2Compute struct {
	prog   glgl.Program
	bb     ms2.Box
	evals  uint64
	invocX int
}

func (sdf *SDF2Compute) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf.prog.Bind()
	defer sdf.prog.Unbind()
	err := glgl.Err()
	if err != nil {
		return fmt.Errorf("binding SDF2Compute program: %w", err)
	}
	err = computeEvaluate(pos, dist, sdf.invocX)
	if err != nil {
		return err
	}
	sdf.evals += uint64(len(pos))
	return nil
}

func (sdf *SDF2Compute) Bounds() ms2.Box {
	return sdf.bb
}

// Evaluations returns total evaluations performed succesfully during sdf's lifetime.
func (sdf *SDF2Compute) Evaluations() uint64 { return sdf.evals }

func elemSize[T any]() int {
	var z T
	return int(unsafe.Sizeof(z))
}

// PolygonGPU implements a direct polygon evaluation via GPU.
type PolygonGPU struct {
	Vertices    []ms2.Vec
	evaluations uint64
	shader      string
	invocX      int
}

func (poly *PolygonGPU) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	return poly.evaluate(pos, dist, userData)
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

// PolygonGPU implements a direct polygon evaluation via GPU.
type Lines2DGPU struct {
	Lines       [][2]ms2.Vec
	Width       float32
	evaluations uint64
	shader      string
	invocX      int
}

func (lines *Lines2DGPU) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	err := lines.evaluate(pos, dist, userData)
	if err == nil {
		lines.evaluations += uint64(len(pos))
	}
	return err
}

func (lines *Lines2DGPU) Bounds() ms2.Box {
	bb := ms2.Box{
		Min: ms2.Vec{X: math32.Inf(1), Y: math32.Inf(1)},
		Max: ms2.Vec{X: math32.Inf(-1), Y: math32.Inf(-1)},
	}
	off := lines.Width / 2
	offp := ms2.Vec{X: off, Y: off}
	offm := ms2.Vec{X: -off, Y: -off}
	for _, v1v2 := range lines.Lines {
		bb.Max = ms2.MaxElem(bb.Max, ms2.Add(v1v2[0], offp))
		bb.Min = ms2.MinElem(bb.Min, ms2.Add(v1v2[0], offm))
		bb.Max = ms2.MaxElem(bb.Max, ms2.Add(v1v2[1], offp))
		bb.Min = ms2.MinElem(bb.Min, ms2.Add(v1v2[1], offm))
	}
	return bb
}

func (lines *Lines2DGPU) Configure(cfg ComputeConfig) error {
	if cfg.InvocX < 1 {
		return errZeroInvoc
	}
	lines.invocX = cfg.InvocX
	lines.shader = fmt.Sprintf(linesshader, cfg.InvocX)
	return nil
}

// winding number from http://geomalgorithms.com/a03-_inclusion.html
const linesshader = `#version 430

layout(local_size_x = %d, local_size_y = 1, local_size_z = 1) in;

// Input: 2D positions at which to evaluate SDF.
layout(std430, binding = 0) buffer PositionsBuffer {
	vec2 vbo_positions[];
};

// Output: Result of SDF evaluation are the distances. Maps to position buffer.
layout(std430, binding = 1) buffer DistancesBuffer {
	float vbo_distances[];
};

layout(std430, binding = 2) buffer LinesBuffer {
	vec4 v[];
};

uniform float WidthOffset;

float segment2( in vec2 p, in vec2 a, in vec2 b )
{
	vec2 pa = p-a, ba = b-a;
	float h = clamp( dot(pa,ba)/dot(ba,ba), 0.0, 1.0 );
	vec2 dv = pa - ba*h;
	return dot(dv, dv);
}

float lines(vec2 p){
	const int num = v.length();
	float d2 = 1.0e23;
	for( int i=0; i<num; i++ )
	{
		vec4 v1v2 = v[i];
		d2 = min(d2, segment2(p, v1v2.xy, v1v2.zw));
	}
	return sqrt(d2)-WidthOffset;
}

void main() {
	int idx = int( gl_GlobalInvocationID.x );

	vec2 p = vbo_positions[idx];     // Get position to evaluate SDF at.
	vbo_distances[idx] = lines(p);   // Evaluate SDF and store to distance buffer.
}
` + "\x00"

// PolygonGPU implements a direct polygon evaluation via GPU.
type DisplaceMulti2D struct {
	Displacements []ms2.Vec
	elemBB        ms2.Box
	evaluations   uint64
	shader        []byte
	invocX        int
}

func (disp *DisplaceMulti2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	err := disp.evaluate(pos, dist, userData)
	if err == nil {
		disp.evaluations += uint64(len(pos))
	}
	return err
}

func (disp *DisplaceMulti2D) Bounds() ms2.Box {
	var bb ms2.Box
	elemBox := disp.elemBB
	for i := range disp.Displacements {
		bb = bb.Union(elemBox.Add(disp.Displacements[i]))
	}
	return bb
}

func (disp *DisplaceMulti2D) Configure(programmer *glbuild.Programmer, element glbuild.Shader2D, cfg ComputeConfig) error {
	if cfg.InvocX < 1 {
		return errZeroInvoc
	}
	var buf bytes.Buffer
	basename, n, err := programmer.WriteSDFDecl(&buf, element)
	if err != nil {
		return err
	} else if n != buf.Len() {
		return errors.New("length written mismatch")
	}
	disp.elemBB = element.Bounds()
	disp.invocX = cfg.InvocX
	disp.shader = fmt.Appendf(disp.shader[:0], multiDisplaceShader, buf.Bytes(), cfg.InvocX, basename)
	return nil
}

// winding number from http://geomalgorithms.com/a03-_inclusion.html
const multiDisplaceShader = `#version 430

%s

layout(local_size_x = %d, local_size_y = 1, local_size_z = 1) in;

// Input: 2D positions at which to evaluate SDF.
layout(std430, binding = 0) buffer PositionsBuffer {
	vec2 vbo_positions[];
};

// Output: Result of SDF evaluation are the distances. Maps to position buffer.
layout(std430, binding = 1) buffer DistancesBuffer {
	float vbo_distances[];
};

layout(std430, binding = 2) buffer DisplacementBuffer {
	vec2 ssbo_displace[];
};

float multidisplace(vec2 p) {
	const int num = ssbo_displace.length();
	float d = 1.0e23;
	for( int i=0; i<num; i++ )
	{
		vec2 pt = p - ssbo_displace[i];
		d = min(d, %s(pt));
	}
	return d;
}


void main() {
	int idx = int( gl_GlobalInvocationID.x );

	vec2 p = vbo_positions[idx];             // Get position to evaluate SDF at.
	vbo_distances[idx] = multidisplace(p);   // Evaluate SDF and store to distance buffer.
}
` + "\x00"
