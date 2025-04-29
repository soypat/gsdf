package glsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/glbuild"
)

//go:embed box3D.glsl
var box3DSrc []byte

// Box3D is the SDF definition for a 3D box:
//
//	float gsdfBox3D(vec3 p, float x, float y, float z, float round)
func Box3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(box3DSrc)
	return obj
}

//go:embed boxframe3D.glsl
var boxframe3DSrc []byte

// BoxFrame3D is the SDF definition for a 3D box frame:
//
//	float gsdfBoxFrame3D(vec3 p, float x, float y, float z, float thick)
func BoxFrame3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(boxframe3DSrc)
	return obj
}

//go:embed cylinder3D.glsl
var cylinder3DSrc []byte

// Cylinder3D is the SDF definition for a 3D circular cylinder:
//
//	float gsdfCylinder3D(vec3 p, float radius, float h, float round)
func Cylinder3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(cylinder3DSrc)
	return obj
}

//go:embed hexagon3D.glsl
var hexagon3DSrc []byte

// Torus3D is the SDF definition for a 3D hexagonal cylinder:
//
//	float gsdfHexagon3D(vec3 p, float side, float extrude)
func Hexagon3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(hexagon3DSrc)
	return obj
}

//go:embed torus3D.glsl
var torus3DSrc []byte

// Hexagon3D is the SDF definition for a 3D torus (toroidal shape):
//
//	float gsdfTorus3D(vec3 p, float t1, float t2)
func Torus3D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(torus3DSrc)
	return obj
}
