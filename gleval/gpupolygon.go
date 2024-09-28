//go:build !tinygo && cgo

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
	ssbo := loadSSBO(poly.Vertices, 2, gl.STATIC_DRAW)
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
