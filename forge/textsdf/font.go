package textsdf

import (
	"errors"
	"fmt"

	"github.com/golang/freetype/truetype"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const firstBasic = '!'
const lastBasic = '~'

// Font implements font parsing and glyph (character) generation.
type Font struct {
	ttf truetype.Font
	gb  truetype.GlyphBuf
	// basicGlyphs optimized array access for common ASCII glyphs.
	basicGlyphs [lastBasic - firstBasic]glyph
	// Other kinds of glyphs.
	otherGlyphs map[rune]glyph
	bld         gsdf.Builder
}

func (f *Font) LoadTTFBytes(ttf []byte) error {
	font, err := truetype.Parse(ttf)
	if err != nil {
		return err
	}
	f.Reset()
	f.ttf = *font
	return nil
}

func (f *Font) Reset() {
	for i := range f.basicGlyphs {
		f.basicGlyphs[i] = glyph{}
	}
	for k := range f.otherGlyphs {
		delete(f.otherGlyphs, k)
	}
}

type glyph struct {
	sdf glbuild.Shader2D
}

func (f *Font) TextLine(s string) (glbuild.Shader2D, error) {
	if len(s) == 0 {
		return nil, errors.New("no text provided")
	}
	var shapes []glbuild.Shader2D
	scale := f.scale()
	var prevChar rune
	for i, c := range s {
		charshape, err := f.Glyph(c)
		if err != nil {
			return nil, fmt.Errorf("char %q: %w", c, err)
		}
		if i > 0 {
			kern := f.ttf.Kern(scale, truetype.Index(prevChar), truetype.Index(c))
			charshape = f.bld.Translate2D(charshape, float32(kern), 0)
		}
		shapes = append(shapes, charshape)
		prevChar = c
	}
	if len(shapes) == 1 {
		return shapes[0], nil
	}
	return f.bld.Union2D(shapes...), nil
}

// Kern returns the horizontal adjustment for the given glyph pair. A positive kern means to move the glyphs further apart.
func (f *Font) Kern(c0, c1 rune) float32 {
	return float32(f.ttf.Kern(f.scale(), truetype.Index(c0), truetype.Index(c1)))
}

// Glyph returns a SDF for a character.
func (f *Font) Glyph(c rune) (_ glbuild.Shader2D, err error) {
	var g glyph
	if c >= firstBasic && c <= lastBasic {
		// Basic ASCII glyph case.
		g = f.basicGlyphs[c-firstBasic]
		if g.sdf == nil {
			// Glyph not yet created. create it.
			g, err = f.makeGlyph(c)
			if err != nil {
				return nil, err
			}
			f.basicGlyphs[c-firstBasic] = g
		}
		return g.sdf, nil
	}
	// Unicode or other glyph.
	g, ok := f.otherGlyphs[c]
	if !ok {
		g, err = f.makeGlyph(c)
		if err != nil {
			return nil, err
		}
		f.otherGlyphs[c] = g
	}
	return g.sdf, nil
}

func (f *Font) scale() fixed.Int26_6 {
	return fixed.Int26_6(f.ttf.FUnitsPerEm())
}

func (f *Font) makeGlyph(char rune) (glyph, error) {
	g := &f.gb
	idx := f.ttf.Index(char)
	scale := f.scale()
	bld := &f.bld
	err := g.Load(&f.ttf, scale, idx, font.HintingNone)
	if err != nil {
		return glyph{}, err
	}

	// Build Glyph.
	shape, fill, err := glyphCurve(bld, g.Points, 0, g.Ends[0])
	if err != nil {
		return glyph{}, err
	} else if !fill {
		return glyph{}, errors.New("first glyph shape is negative space")
	}
	start := g.Ends[0]
	g.Ends = g.Ends[1:]
	for _, end := range g.Ends {
		sdf, fill, err := glyphCurve(bld, g.Points, start, end)
		start = end
		if err != nil {
			return glyph{}, err
		}
		if fill {
			shape = bld.Union2D(shape, sdf)
		} else {
			shape = bld.Difference2D(shape, sdf)
		}
	}

	return glyph{sdf: shape}, nil
}

func glyphCurve(bld *gsdf.Builder, points []truetype.Point, start, end int) (glbuild.Shader2D, bool, error) {
	var (
		sampler = ms2.Spline3Sampler{Spline: quadBezier, Tolerance: 0.1}
		sum     float32
	)

	n := end - start
	i := start
	var poly []ms2.Vec
	vPrev := p2v(points[end-1])
	for i < start+n {
		p0, p1, p2 := points[i], points[start+(i+1)%n], points[start+(i+2)%n]
		onBits := p0.Flags&1 |
			(p1.Flags&1)<<1 |
			(p2.Flags&1)<<2
		v0, v1, v2 := p2v(p0), p2v(p1), p2v(p2)
		implicit0 := ms2.Scale(0.5, ms2.Add(v0, v1))
		implicit1 := ms2.Scale(0.5, ms2.Add(v1, v2))
		switch onBits {
		case 0b010, 0b110:
			// sampler.SetSplinePoints(vPrev, v0, v1, ms2.Vec{})
			i += 1
			println("prohibited")
			// not valid off start. If getting this error try replacing with `i++;continue`
			// return nil, false, errors.New("invalid start to bezier")
			poly = append(poly, v0)
			continue
			// // if i == start+n-1 {
			// // 	poly = append(poly, v0)
			// // }
			// vPrev = v0
			// i += 1
			// return bld.NewCircle(1), sum > 0, nil
			// continue
		case 0b000:
			// implicit-off-implicit.
			sampler.SetSplinePoints(implicit0, v1, implicit1, ms2.Vec{})
			v0 = implicit0
			i += 1
		case 0b001:
			// on-off-implicit.
			sampler.SetSplinePoints(v0, v1, implicit1, ms2.Vec{})
			i += 1
		case 0b011, 0b111:
			// on-on Straight line.
			poly = append(poly, v0)
			i += 1
			sum += (v0.X - vPrev.X) * (v0.Y + vPrev.Y)
			vPrev = v0
			continue
		case 0b100:
			// implicit-off-on.
			sampler.SetSplinePoints(implicit0, v1, v2, ms2.Vec{})
			v0 = implicit0
			i += 2
		case 0b101:
			// On-off-on.
			sampler.SetSplinePoints(v0, v1, v2, ms2.Vec{})
			i += 2
		}
		poly = append(poly, v0) // Append start point.
		poly = sampler.SampleBisect(poly, 1)
		sum += (v0.X - vPrev.X) * (v0.Y + vPrev.Y)
		vPrev = v0
	}
	return bld.NewPolygon(poly), sum > 0, bld.Err()
}

func p2v(p truetype.Point) ms2.Vec {
	return ms2.Vec{
		X: float32(p.X),
		Y: float32(p.Y),
	}
}

var quadBezier = ms2.NewSpline3([]float32{
	1, 0, 0, 0,
	-2, 2, 0, 0,
	1, -2, 1, 0,
	0, 0, 0, 0,
})
