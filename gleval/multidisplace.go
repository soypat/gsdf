package gleval

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf/glbuild"
)

// PolygonGPU implements a direct polygon evaluation via GPU.
type DisplaceMulti2D struct {
	Displacements []ms2.Vec
	elemBB        ms2.Box
	evaluations   uint64
	shader        []byte
	invocX        int
}

func (lines *DisplaceMulti2D) Bounds() ms2.Box {
	var bb ms2.Box
	elemBox := lines.elemBB
	for i := range lines.Displacements {
		bb = bb.Union(elemBox.Add(lines.Displacements[i]))
	}
	return bb
}

func (lines *DisplaceMulti2D) Configure(programmer *glbuild.Programmer, element glbuild.Shader2D, cfg ComputeConfig) error {
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
	lines.elemBB = element.Bounds()
	lines.invocX = cfg.InvocX
	lines.shader = fmt.Appendf(lines.shader[:0], multiDisplaceShader, buf.Bytes(), cfg.InvocX, basename)
	return nil
}

func (lines *DisplaceMulti2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	} else if len(lines.shader) == 0 {
		return errors.New("need to initialize LinesGPU before first use")
	}
	cmp := unsafe.String(&lines.shader[0], len(lines.shader))
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: cmp})
	if err != nil {
		return fmt.Errorf("compiling GL program: %w", err)
	}
	defer prog.Delete()
	prog.Bind()

	err = glgl.Err()
	if err != nil {
		return fmt.Errorf("binding LinesGPU program: %w", err)
	}
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadSSBO(lines.Displacements, 2, gl.STATIC_DRAW)
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
