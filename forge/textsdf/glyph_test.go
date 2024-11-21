package textsdf

import (
	"testing"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

func TestABC(t *testing.T) {
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
	sampler := ms2.Spline3Sampler{
		Spline:    ms2.SplineBezier(),
		Tolerance: 0.01,
	}

	for i := 0; i < len(spline); i += 4 {
		v0, v1, v2, v3 := spline[4*i], spline[4*i+1], spline[4*i+2], spline[4*i+3]

		shape = bld.Union2D(shape, bld.Translate2D(circle, v1.X, v1.Y))
		shape = bld.Union2D(shape, bld.Translate2D(circle, v2.X, v2.Y))

		sampler.SetSplinePoints(v0, v1, v2, v3)
		points := sampler.SampleBisectWithExtremes(nil, 2)
		for _, pt := range points {
			poly.Add(pt)
		}
	}
	v, err := poly.AppendVecs(nil)
	if err != nil {
		t.Fatal(err)
	}

	shape = bld.Union2D(shape, bld.NewPolygon(v))
	_ = shape
	sdfcpu, err := gleval.NewCPUSDF2(shape)
	if err != nil {
		t.Fatal(err)
	}
	err = gsdfaux.RenderPNGFile("shape.png", sdfcpu, 512, nil)
	if err != nil {
		t.Fatal(err)
	}
}
