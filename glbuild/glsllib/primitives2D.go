package glsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/glbuild"
)

//go:embed ellipse2D.glsl
var ellipseSrc []byte

// Ellipse2D is the SDF definition for a 2D ellipse:
//
//	float gsdfEllipse2D(vec2 p, float a, float b)
func Ellipse2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(ellipseSrc)
	return obj
}

//go:embed eqtri2D.glsl
var eqTriSrc []byte

// EquilateralTriangle2D is the SDF definition for a 2D equilateral triangle:
//
//	float gsdfEqTri2D(vec2 p, float h)
func EquilateralTriangle2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(eqTriSrc)
	return obj
}

//go:embed rect2D.glsl
var rect2DSrc []byte

// Rectangle2D is the SDF definition for a 2D rectangle:
//
//	float gsdfRect2D(vec2 p, float x, float y)
func Rectangle2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(rect2DSrc)
	return obj
}

//go:embed octagon2D.glsl
var oct2DSrc []byte

// Octagon2D is the SDF definition for a 2D octagon:
//
//	float gsdfOctagon2D(vec2 p, float r)
func Octagon2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(oct2DSrc)
	return obj
}

//go:embed hexagon2D.glsl
var hex2DSrc []byte

// Octagon2D is the SDF definition for a 2D octagon:
//
//	float gsdfHexagon2D(vec2 p, float r)
func Hexagon2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(hex2DSrc)
	return obj
}

//go:embed arc2D.glsl
var arc2DSrc []byte

// Arc2D is the SDF definition for a 2D arc:
//
//	float gsdfArc2D(vec2 p, float r, float t, float sinAngle, float cosAngle)
func Arc2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(arc2DSrc)
	return obj
}

//go:embed bezierQ2D.glsl
var bezierQ2DSrc []byte

// QuadraticBezier2D is the SDF definition for a 2D quadratic bezier:
//
//	float gsdfBezierQ2D(vec2 p, vec2 A, vec2 B, vec2 C, float thick)
func QuadraticBezier2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(bezierQ2DSrc)
	return obj
}

//go:embed diamond2D.glsl
var diamond2DSrc []byte

// Diamond2D is the SDF definition for a 2D diamond shape:
//
//	float gsdfDiamond2D(vec2 p, float x, float y)
func Diamond2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(diamond2DSrc)
	return obj
}

//go:embed roundedX2D.glsl
var x2DSrc []byte

// RoundedX2D is the SDF definition for a 2D rounded X:
//
//	float gsdfRoundedX2D(vec2 p, float w, float r)
func RoundedX2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(x2DSrc)
	return obj
}
