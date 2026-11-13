package wgsllib

import (
	_ "embed"

	"github.com/soypat/gsdf/glbuild"
)

//go:embed ellipse2D.wgsl
var ellipseSrc []byte

// Ellipse2D is the SDF definition for a 2D ellipse:
//
//	fn gsdfEllipse2D(p: vec2f, a: f32, b: f32) -> f32
func Ellipse2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(ellipseSrc)
	return obj
}

//go:embed eqtri2D.wgsl
var eqTriSrc []byte

// EquilateralTriangle2D is the SDF definition for a 2D equilateral triangle:
//
//	fn gsdfEqTri(p: vec2f, h: f32) -> f32
func EquilateralTriangle2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(eqTriSrc)
	return obj
}

//go:embed rect2D.wgsl
var rect2DSrc []byte

// Rectangle2D is the SDF definition for a 2D rectangle:
//
//	fn gsdfRect2D(p: vec2f, x: f32, y: f32) -> f32
func Rectangle2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(rect2DSrc)
	return obj
}

//go:embed rectRound2D.wgsl
var rectRound2DSrc []byte

// RectangleRound2D is the SDF definition for a 2D rectangle with rounded corners:
//
//	fn gsdfRectRound2D(p: vec2f, x: f32, y: f32, r: f32) -> f32
func RectangleRound2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(rectRound2DSrc)
	return obj
}

//go:embed octagon2D.wgsl
var oct2DSrc []byte

// Octagon2D is the SDF definition for a 2D octagon:
//
//	fn gsdfOctagon2D(p: vec2f, r: f32) -> f32
func Octagon2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(oct2DSrc)
	return obj
}

//go:embed hexagon2D.wgsl
var hex2DSrc []byte

// Octagon2D is the SDF definition for a 2D octagon:
//
//	fn gsdfHexagon2D(p: vec2f, r: f32) -> f32
func Hexagon2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(hex2DSrc)
	return obj
}

//go:embed arc2D.wgsl
var arc2DSrc []byte

// Arc2D is the SDF definition for a 2D arc:
//
//	fn gsdfArc2D(p: vec2f, r: f32, t: f32, sinAngle: f32, cosAngle: f32) -> f32
func Arc2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(arc2DSrc)
	return obj
}

//go:embed bezierQ2D.wgsl
var bezierQ2DSrc []byte

// QuadraticBezier2D is the SDF definition for a 2D quadratic bezier:
//
//	fn gsdfBezierQ2D(p: vec2f, A: vec2f, B: vec2f, C: vec2f, thick: f32) -> f32
func QuadraticBezier2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(bezierQ2DSrc)
	return obj
}

//go:embed diamond2D.wgsl
var diamond2DSrc []byte

// Diamond2D is the SDF definition for a 2D diamond shape:
//
//	fn gsdfDiamond2D(p: vec2f, x: f32, y: f32) -> f32
func Diamond2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(diamond2DSrc)
	return obj
}

//go:embed roundedX2D.wgsl
var x2DSrc []byte

// RoundedX2D is the SDF definition for a 2D rounded X:
//
//	fn gsdfRoundedX2D(p: vec2f, w: f32, r: f32) -> f32
func RoundedX2D() glbuild.ShaderObject {
	obj, _ := glbuild.MakeShaderFunction(x2DSrc)
	return obj
}
