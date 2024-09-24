//go:build !tinygo && cgo

package gleval

import (
	"errors"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/v4.6-core/glgl"
)

// PolygonGPU implements a direct polygon evaluation via GPU.
type PolygonGPU struct {
	Vertices    []ms2.Vec
	evaluations uint64
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

func (poly *PolygonGPU) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	}
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: polyshader})
	if err != nil {
		return err
	}
	defer prog.Delete()
	tex, _, err := loadTexture(poly.Vertices, 2, glgl.ReadOnly)
	if err != nil {
		return err
	}
	defer tex.Delete()
	err = computeEvaluate(prog, pos, dist)
	if err != nil {
		return err
	}
	poly.evaluations += uint64(len(dist))
	return nil
}

// winding number from http://geomalgorithms.com/a03-_inclusion.html
const polyshader = `#version 430
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
	bvec3 cond = bvec3( p.y>=v[i].y, 
						p.y <v[j].y, 
						e.x*w.y>e.y*w.x );
	if( all(cond) || all(not(cond)) ) s=-s;  
}
return s*sqrt(d);
}

layout(local_size_x = 1, local_size_y = 1, local_size_z = 1) in;
layout(rg32f, binding = 0) uniform image2D in_tex;
// The binding argument refers to the textures Unit.
layout(r32f, binding = 1) uniform image2D out_tex;

void main() {
	// get position to read/write data from.
	ivec2 pos = ivec2( gl_GlobalInvocationID.xy );
	// Get SDF position value.
	vec2 p = imageLoad( in_tex, pos ).rg;
	float distance = poly(p);
	// store new value in image
	imageStore( out_tex, pos, vec4( distance, 0.0, 0.0, 0.0 ) );
}
` + "\x00"
