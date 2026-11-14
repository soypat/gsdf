package wgsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/gpusdf/glbuild"
)

//go:embed box3D.wgsl
var box3DSrc []byte

// Box3D is the SDF definition for a 3D box:
//
//	fn gsdfBox3D(p: vec3f, x: f32, y: f32, z: f32, rnd: f32) -> f32
func Box3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(box3DSrc)
	return obj
}

//go:embed boxframe3D.wgsl
var boxframe3DSrc []byte

// BoxFrame3D is the SDF definition for a 3D box frame:
//
//	fn gsdfBoxFrame3D(p: vec3f, x: f32, y: f32, z: f32, thick: f32) -> f32
func BoxFrame3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(boxframe3DSrc)
	return obj
}

//go:embed cylinder3D.wgsl
var cylinder3DSrc []byte

// Cylinder3D is the SDF definition for a 3D circular cylinder:
//
//	fn gsdfCylinder3D(p: vec3f, radius: f32, h: f32, rnd: f32) -> f32
func Cylinder3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(cylinder3DSrc)
	return obj
}

//go:embed hexagon3D.wgsl
var hexagon3DSrc []byte

// Torus3D is the SDF definition for a 3D hexagonal cylinder:
//
//	fn gsdfHexagon3D(p: vec3f, side: f32, extrude: f32) -> f32
func Hexagon3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(hexagon3DSrc)
	return obj
}

//go:embed torus3D.wgsl
var torus3DSrc []byte

// Hexagon3D is the SDF definition for a 3D torus (toroidal shape):
//
//	fn gsdfTorus3D(p: vec3f, t1: f32, t2: f32) -> f32
func Torus3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(torus3DSrc)
	return obj
}
