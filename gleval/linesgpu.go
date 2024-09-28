package gleval

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/chewxy/math32"
	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/v4.6-core/glgl"
)

// PolygonGPU implements a direct polygon evaluation via GPU.
type Lines2DGPU struct {
	Lines       [][2]ms2.Vec
	Width       float32
	evaluations uint64
	shader      string
	invocX      int
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

func (lines *Lines2DGPU) Evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	} else if lines.shader == "" {
		return errors.New("need to initialize LinesGPU before first use")
	}
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: lines.shader})
	if err != nil {
		return fmt.Errorf("compiling GL program: %w", err)
	}
	defer prog.Delete()
	prog.Bind()
	prog.SetUniform1f("WidthOffset\x00", lines.Width/2)
	err = glgl.Err()
	if err != nil {
		return fmt.Errorf("binding LinesGPU program: %w", err)
	}
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadSSBO(lines.Lines, 2, gl.STATIC_DRAW)
	err = glgl.Err()
	if err != nil {
		return err
	}
	p.Pin(&ssbo)
	defer p.Unpin()
	defer gl.DeleteBuffers(1, &ssbo)

	err = computeEvaluate(pos, dist, lines.invocX)
	if err != nil {
		return err
	}
	lines.evaluations += uint64(len(dist))
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
