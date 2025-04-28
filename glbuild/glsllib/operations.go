package glsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/glbuild"
)

//go:embed circarray2D.glsl
var circarray2DSrc []byte

// PartialCircArray2D is partial logic for a circular array SDF implementation:
//
//	vec4 gsdfPartialCircArray2D(vec2 p, float ncirc, float angle, float ninsm1)
func PartialCircArray2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(circarray2DSrc)
	return obj
}
