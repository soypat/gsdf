package textsdf

import (
	"testing"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
)

type glyph struct {
}

func TestABC(t *testing.T) {
	bz := ms2.SplineBezier()
	spline := []ms2.Vec{
		{0, 0},
		{1, 0},
		{1, 0},
		{1, 1},
	}
	// var points []ms2.Vec
	var bld gsdf.Builder
	var poly ms2.PolygonBuilder
	circle := bld.NewCircle(0.1)
	shape := bld.Translate2D(circle, spline[0].X, spline[1].Y)
	for i := 0; i < len(spline); i += 4 {
		v0, v1, v2, v3 := spline[4*i], spline[4*i+1], spline[4*i+2], spline[4*i+3]
		shape = bld.Union2D(shape, bld.Translate2D(circle, v1.X, v1.Y))
		shape = bld.Union2D(shape, bld.Translate2D(circle, v2.X, v2.Y))
		for x := float32(0.0); x < 1; x += 1. / 64 {
			vx := bz.Evaluate(x, v0, v1, v2, v3)
			poly.Add(vx)
		}
	}
	v, err := poly.AppendVecs(nil)
	if err != nil {
		t.Fatal(err)
	}

	shape = bld.Union2D(shape, bld.NewPolygon(v))
	// _ = shape
	// sdfcpu, err := gleval.NewCPUSDF2(shape)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// err = gsdfaux.RenderPNGFile("shape.png", sdfcpu, 512, nil)
	// if err != nil {
	// 	t.Fatal(err)
	// }
}
