package textsdf

import (
	"errors"
	"fmt"
	"unicode"

	"github.com/golang/freetype/truetype"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const firstBasic = '!'
const lastBasic = '~'

type FontConfig struct {
	// RelativeGlyphTolerance sets the permissible curve tolerance for glyphs. Must be between 0..1. If zero a reasonable value is chosen.
	RelativeGlyphTolerance float32
}

// Font implements font parsing and glyph (character) generation.
type Font struct {
	ttf truetype.Font
	gb  truetype.GlyphBuf
	// basicGlyphs optimized array access for common ASCII glyphs.
	basicGlyphs [lastBasic - firstBasic + 1]glyph
	// Other kinds of glyphs.
	otherGlyphs map[rune]glyph
	bld         gsdf.Builder
	reltol      float32 // Set by config or reset call if zeroed.
}

func (f *Font) Configure(cfg FontConfig) error {
	if cfg.RelativeGlyphTolerance < 0 || cfg.RelativeGlyphTolerance >= 1 {
		return errors.New("invalid RelativeGlyphTolerance")
	}
	f.reset()
	f.reltol = cfg.RelativeGlyphTolerance
	return nil
}

// LoadTTFBytes loads a TTF file blob into f. After calling Load the Font is ready to generate text SDFs.
func (f *Font) LoadTTFBytes(ttf []byte) error {
	font, err := truetype.Parse(ttf)
	if err != nil {
		return err
	}
	f.reset()
	f.ttf = *font
	return nil
}

// reset resets most internal state of Font without removing underlying assigned font.
func (f *Font) reset() {
	for i := range f.basicGlyphs {
		f.basicGlyphs[i] = glyph{}
	}
	if f.otherGlyphs == nil {
		f.otherGlyphs = make(map[rune]glyph)
	} else {
		for k := range f.otherGlyphs {
			delete(f.otherGlyphs, k)
		}
	}
	if f.reltol == 0 {
		f.reltol = 0.15
	}
}

type glyph struct {
	sdf glbuild.Shader2D
}

// TextLine returns a single line of text with the set font.
// TextLine takes kerning and advance width into account for letter spacing.
// Glyph locations are set starting at x=0 and appended in positive x direction.
func (f *Font) TextLine(s string) (glbuild.Shader2D, error) {
	var shapes []glbuild.Shader2D
	scale := f.scale()
	var idxPrev truetype.Index
	var xOfs int64
	scalout := f.scaleout()
	for ic, c := range s {
		if !unicode.IsGraphic(c) {
			return nil, fmt.Errorf("char %q not graphic", c)
		}

		idx := truetype.Index(c)
		hm := f.ttf.HMetric(scale, idx)
		if unicode.IsSpace(c) {
			if c == '\t' {
				hm.AdvanceWidth *= 4
			}
			xOfs += int64(hm.AdvanceWidth)
			continue
		}
		charshape, err := f.Glyph(c)
		if err != nil {
			return nil, fmt.Errorf("char %q: %w", c, err)
		}

		kern := f.ttf.Kern(scale, idxPrev, idx)
		xOfs += int64(kern)
		idxPrev = idx
		if ic == 0 {
			xOfs += int64(hm.LeftSideBearing)
		}
		charshape = f.bld.Translate2D(charshape, float32(xOfs)*scalout, 0)
		shapes = append(shapes, charshape)
		xOfs += int64(hm.AdvanceWidth)
	}
	if len(shapes) == 1 {
		return shapes[0], nil
	} else if len(shapes) == 0 {
		// Only whitespace.
		return nil, errors.New("no text provided")
	}
	return f.bld.Union2D(shapes...), nil
}

// Kern returns the horizontal adjustment for the given glyph pair. A positive kern means to move the glyphs further apart.
func (f *Font) Kern(c0, c1 rune) float32 {
	return float32(f.ttf.Kern(f.scale(), truetype.Index(c0), truetype.Index(c1)))
}

// Kern returns the horizontal adjustment for the given glyph pair. A positive kern means to move the glyphs further apart.
func (f *Font) AdvanceWidth(c rune) float32 {
	return float32(f.ttf.HMetric(f.scale(), truetype.Index(c)).AdvanceWidth)
}

// Glyph returns a SDF for a character defined by the argument rune.
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

func (f *Font) rawbounds() ms2.Box {
	bb := f.ttf.Bounds(f.scale())
	return ms2.Box{
		Min: ms2.Vec{X: float32(bb.Min.X), Y: float32(bb.Min.Y)},
		Max: ms2.Vec{X: float32(bb.Max.X), Y: float32(bb.Max.Y)},
	}
}

// scaleout defines the scaling from fixed point integers to
func (f *Font) scaleout() float32 {
	bb := f.rawbounds()
	sz := bb.Size().Min()
	return 1. / float32(sz)
}

func (f *Font) makeGlyph(char rune) (glyph, error) {
	g := &f.gb
	bld := &f.bld

	idx := f.ttf.Index(char)
	scale := f.scale()
	// hm := f.ttf.HMetric(scale, idx)
	err := g.Load(&f.ttf, scale, idx, font.HintingNone)
	if err != nil {
		return glyph{}, err
	}
	scaleout := f.scaleout()

	tol := f.reltol
	// Build Glyph.
	shape, fill, err := glyphCurve(bld, g.Points, 0, g.Ends[0], tol, scaleout)
	if err != nil {
		return glyph{}, err
	} else if !fill {
		_ = fill // This is not an error...
		// return glyph{}, errors.New("first glyph shape is negative space")
	}
	start := g.Ends[0]
	g.Ends = g.Ends[1:]
	for _, end := range g.Ends {
		sdf, fill, err := glyphCurve(bld, g.Points, start, end, tol, scaleout)
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

func glyphCurve(bld *gsdf.Builder, points []truetype.Point, start, end int, tol, scale float32) (glbuild.Shader2D, bool, error) {
	var (
		sampler = ms2.Spline3Sampler{Spline: quadBezier, Tolerance: tol}
		sum     float32
	)
	points = points[start:end]
	n := len(points)
	i := 0
	var poly []ms2.Vec
	vPrev := p2v(points[n-1], scale)
	for i < n {
		p0, p1, p2 := points[i], points[(i+1)%n], points[(i+2)%n]
		onBits := onbits3(points, 0, n, i)
		v0, v1, v2 := p2v(p0, scale), p2v(p1, scale), p2v(p2, scale)
		implicit0 := ms2.Scale(0.5, ms2.Add(v0, v1))
		implicit1 := ms2.Scale(0.5, ms2.Add(v1, v2))
		switch onBits {
		case 0b010, 0b110:
			// implicit off start case?
			fallthrough
		case 0b011, 0b111:
			// on-on Straight line.
			poly = append(poly, v0)
			i += 1
			sum += (v0.X - vPrev.X) * (v0.Y + vPrev.Y)
			vPrev = v0
			continue

		case 0b000:
			// implicit-off-implicit.
			sampler.SetSplinePoints(implicit0, v1, implicit1, ms2.Vec{})
			v0 = implicit0
			i += 1

		case 0b001:
			// on-off-implicit.
			sampler.SetSplinePoints(v0, v1, implicit1, ms2.Vec{})
			i += 1

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
		poly = sampler.SampleBisect(poly, 4)
		sum += (v0.X - vPrev.X) * (v0.Y + vPrev.Y)
		vPrev = v0
	}
	return bld.NewPolygon(poly), sum > 0, bld.Err()
}

func p2v(p truetype.Point, scale float32) ms2.Vec {
	return ms2.Vec{
		X: float32(p.X) * scale,
		Y: float32(p.Y) * scale,
	}
}

var quadBezier = ms2.NewSpline3([]float32{
	1, 0, 0, 0,
	-2, 2, 0, 0,
	1, -2, 1, 0,
	0, 0, 0, 0,
})

func onbits3(points []truetype.Point, start, end, i int) uint32 {
	n := end - start
	p0, p1, p2 := points[i], points[start+(i+1)%n], points[start+(i+2)%n]
	return p0.Flags&1 |
		(p1.Flags&1)<<1 |
		(p2.Flags&1)<<2
}
