package wgsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/glbuild"
)

//go:embed winding.wgsl
var windingSrc []byte

// WindingNumber is a winding number implementation for polygon SDF calculation.
//
//	fn gsdfWinding(p: vec2f, v1: vec2f, v2: vec2f, d_s: vec2f) -> vec2f
func WindingNumber() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(windingSrc)
	return obj
}

//go:embed linesq2D.wgsl
var line2DSrc []byte

// LineSquared2D is the SDF definition for a single 2D line (distance squared for performance reasons):
//
//	fn gsdfLineSq2D(p: vec2f, v1v2: vec4f) -> f32
func LineSquared2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(line2DSrc)
	return obj
}
