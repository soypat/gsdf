package textsdf

import (
	"testing"

	_ "embed"

	"github.com/golang/freetype/truetype"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

//go:embed iso-3098.ttf
var _isonormTTF []byte

func TestABC(t *testing.T) {
	ttf, err := truetype.Parse(_isonormTTF)
	if err != nil {
		t.Fatal(err)
	}
	var glyph truetype.GlyphBuf
	shape, err := sdf(ttf, &glyph, 'A')
	if err != nil {
		t.Fatal(err)
	}
	sdfcpu, err := gleval.NewCPUSDF2(shape)
	if err != nil {
		t.Fatal(err)
	}
	err = gsdfaux.RenderPNGFile("shape.png", sdfcpu, 512, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func sdf(ttf *truetype.Font, glyph *truetype.GlyphBuf, char rune) (glbuild.Shader2D, error) {
	idx := ttf.Index(char)
	scale := fixed.Int26_6(ttf.FUnitsPerEm())
	// hm := ttf.HMetric(scale, idx)

	// kern := ttf.Kern(scale, iprev, idx)
	// xOfs := float32(kern)
	err := glyph.Load(ttf, scale, idx, font.HintingNone)
	if err != nil {
		return nil, err
	}
	return glyphSDF(glyph)

}

func glyphSDF(g *truetype.GlyphBuf) (glbuild.Shader2D, error) {
	var spline []ms2.Vec
	var bld gsdf.Builder
	shape := bld.NewCircle(0.1)
	for n := range g.Ends {
		spline = spline[:0]
		start := 0
		if n != 0 {
			start = g.Ends[n-1]
		}
		end := g.Ends[n] - 1
		if n != 1 {
			continue
		}
		sdf, cw, err := glyphCurve(g.Points, start, end)
		if err != nil {
			return shape, err
		}
		cw = true
		if cw {
			shape = bld.Union2D(shape, sdf)
		} else {
			shape = bld.Difference2D(shape, sdf)
		}

	}
	return shape, nil
}

// func glyphCurve2(points []truetype.Point, start, end int) (glbuild.Shader2D, bool, error) {
// 	var (
// 		bld     gsdf.Builder
// 		sampler = ms2.Spline3Sampler{Spline: quadraticBezier(), Tolerance: 2}
// 		sum     float32
// 	)
// 	n := end - start + 1
// 	i := 0
// 	vprev := p2v(points[end])
// 	for i < n {
// 		p0, p1, p2 := points[i], points[start+(i+1)%n], points[start+(i+2)%n]
// 		onBits := p0.Flags&1 |
// 			(p1.Flags&1)<<1 |
// 			(p2.Flags&1)<<2
// 		switch bits {
// 		case 0b001:
// 			sampler.SetSplinePoints(vprev,)
// 		}
// 		off := points[i].Flags&1 == 0 // Point on or off curve.

// 	}
// }

func glyphCurve(points []truetype.Point, start, end int) (glbuild.Shader2D, bool, error) {
	var bld gsdf.Builder
	var spline []ms2.Vec
	sampler := ms2.Spline3Sampler{
		Spline:    quadraticBezier(),
		Tolerance: 2,
	}
	var sum float32
	offPrev := points[end].Flags&1 == 0
	vPrev := p2v(points[end])
	for i := start; i <= end; i++ {
		p := points[i]
		v := p2v(p)
		off := p.Flags&1 == 0 // Point on or off curve.
		if off && offPrev {
			// Implicit point on curve as midpoint of 2 off-points.
			spline = append(spline, ms2.Scale(0.5, ms2.Add(v, vPrev)))
		} else if !off && !offPrev {
			// Add inneffective bezier off point.
			spline = append(spline, ms2.Scale(0.5, ms2.Add(v, vPrev)))
		}
		spline = append(spline, v)
		sum += (v.X - vPrev.X) * (v.Y + vPrev.Y)
		vPrev = v
		offPrev = off
	}
	cw := sum > 0
	var poly []ms2.Vec
	const c = 10
	circle := bld.NewCircle(c / 2)
	CIRCLE := bld.NewCircle(c)
	shape := bld.NewCircle(0.1)
	for i := 0; i+2 < len(spline); i += 2 {
		p0, cp, p1 := spline[i], spline[i+1], spline[i+2]
		sampler.SetSplinePoints(p0, cp, p1, ms2.Vec{})
		shape = bld.Union2D(shape, bld.Translate2D(CIRCLE, p0.X, p0.Y), bld.Translate2D(circle, cp.X, cp.Y))
		poly = append(poly,
			sampler.Evaluate(0),
			sampler.Evaluate(0.25),
			sampler.Evaluate(0.5),
			sampler.Evaluate(0.75),
		)
		_ = p1
		_ = cp
	}
	return shape, cw, nil
	sdf := bld.NewPolygon(poly)
	return sdf, cw, nil
}

func p2v(p truetype.Point) ms2.Vec {
	return ms2.Vec{
		X: float32(p.X),
		Y: float32(p.Y),
	}
}

func quadraticBezier() (s ms2.Spline3) {
	matrix := []float32{
		1, 0, 0, 0,
		-2, 2, 0, 0,
		1, -2, 1, 0,
		0, 0, 0, 0,
	}
	// for i := range matrix {
	// 	matrix[i] *= 20
	// }
	return ms2.NewSpline3(matrix)
	// return ms2.NewSpline3([]float32{
	// 	1, -2, 1, 0,
	// 	0, 2, -2, 0,
	// 	0, 0, 1, 0,
	// 	0, 0, 0, 0,
	// })
}
