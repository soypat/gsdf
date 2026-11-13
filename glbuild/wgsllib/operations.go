package wgsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/glbuild"
)

//go:embed circarray2D.wgsl
var circarray2DSrc []byte

// PartialCircArray2D is partial logic for a circular array SDF implementation:
//
//	fn gsdfPartialCircArray2D(p: vec2f, ncirc: f32, angle: f32, ninsm1: f32) -> vec4f
func PartialCircArray2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(circarray2DSrc)
	return obj
}
