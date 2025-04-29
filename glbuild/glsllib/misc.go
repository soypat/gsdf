package glsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/glbuild"
)

//go:embed winding.glsl
var windingSrc []byte

// WindingNumber is a winding number implementation for polygon SDF calculation.
//
//	vec2 gsdfWinding(vec2 p, vec2 v1, vec2 v2)
func WindingNumber() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(windingSrc)
	return obj
}

//go:embed linesq2D.glsl
var line2DSrc []byte

// LineSquared2D is the SDF definition for a single 2D line (distance squared for performance reasons):
//
//	float gsdfLineSq2D(vec2 p, vec4 v1v2)
func LineSquared2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(line2DSrc)
	return obj
}
