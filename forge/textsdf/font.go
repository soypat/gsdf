package textsdf

import (
	"errors"
	"fmt"
	"unicode"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const firstBasic = '!'
const lastBasic = '~'

var defaultBuilder = &gsdf.Builder{}

type FontConfig struct {
	// RelativeGlyphTolerance sets the permissible curve tolerance for glyphs. Must be between 0..1. If zero a reasonable value is chosen.
	RelativeGlyphTolerance float32
	Builder                *gsdf.Builder
}

// Font implements font parsing and glyph (character) generation.
type Font struct {
	buf sfnt.Buffer
	sfn *sfnt.Font
	// basicGlyphs optimized array access for common ASCII glyphs.
	basicGlyphs [lastBasic - firstBasic + 1]glyph
	// Other kinds of glyphs.
	otherGlyphs map[rune]*glyph
	bld         *gsdf.Builder
	reltol      float32 // Set by config or reset call if zeroed.
}

func (f *Font) Configure(cfg FontConfig) error {
	if cfg.RelativeGlyphTolerance < 0 || cfg.RelativeGlyphTolerance >= 1 {
		return errors.New("invalid RelativeGlyphTolerance")
	}
	f.reset()
	f.reltol = cfg.RelativeGlyphTolerance
	if cfg.Builder != nil {
		f.bld = cfg.Builder
	}
	return nil
}

// LoadTTFBytes loads a TTF file blob into f. After calling Load the Font is ready to generate text SDFs.
func (f *Font) LoadTTFBytes(ttf []byte) error {
	font, err := sfnt.Parse(ttf)
	if err != nil {
		return err
	}
	f.reset()
	f.sfn = font
	return nil
}

// reset resets most internal state of Font without removing underlying assigned font.
func (f *Font) reset() {
	for i := range f.basicGlyphs {
		f.basicGlyphs[i] = glyph{}
	}
	if f.otherGlyphs == nil {
		f.otherGlyphs = make(map[rune]*glyph)
	} else {
		for k := range f.otherGlyphs {
			delete(f.otherGlyphs, k)
		}
	}
	if f.reltol == 0 {
		f.reltol = 0.15
	}
	if f.bld == nil {
		f.bld = defaultBuilder
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
	ppem := f.scale()

	var idxPrev sfnt.GlyphIndex
	var xOfs fixed.Int26_6
	scaleout := f.scaleout()
	for ic, c := range s {
		if !unicode.IsGraphic(c) {
			return nil, fmt.Errorf("char %q not graphic", c)
		}

		idx, err := f.sfn.GlyphIndex(&f.buf, c)
		if err != nil {
			return nil, fmt.Errorf("char %q glyph index: %w", c, err)
		}

		advance, err := f.sfn.GlyphAdvance(&f.buf, idx, ppem, font.HintingNone)
		if err != nil {
			return nil, fmt.Errorf("char %q advance: %w", c, err)
		}

		if unicode.IsSpace(c) {
			if c == '\t' {
				advance *= 4
			}
			xOfs += advance
			continue
		}

		charshape, err := f.Glyph(c)
		if err != nil {
			return nil, fmt.Errorf("char %q: %w", c, err)
		}

		if ic > 0 {
			kern, _ := f.sfn.Kern(&f.buf, idxPrev, idx, ppem, font.HintingNone)
			xOfs += kern
		}
		idxPrev = idx

		charshape = f.bld.Translate2D(charshape, float32(xOfs)*scaleout, 0)
		shapes = append(shapes, charshape)
		xOfs += advance
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
	idx0, _ := f.sfn.GlyphIndex(&f.buf, c0)
	idx1, _ := f.sfn.GlyphIndex(&f.buf, c1)
	kern, _ := f.sfn.Kern(&f.buf, idx0, idx1, f.scale(), font.HintingNone)
	return float32(kern) * f.scaleout()
}

// AdvanceWidth returns the horizontal advance width for the given glyph.
func (f *Font) AdvanceWidth(c rune) float32 {
	idx, _ := f.sfn.GlyphIndex(&f.buf, c)
	advance, _ := f.sfn.GlyphAdvance(&f.buf, idx, f.scale(), font.HintingNone)
	return float32(advance) * f.scaleout()
}

// Glyph returns a SDF for a character defined by the argument rune.
func (f *Font) Glyph(c rune) (_ glbuild.Shader2D, err error) {
	g, err := f.glyph(c)
	if err != nil {
		return nil, err
	}
	return g.sdf, nil
}

func (f *Font) glyph(c rune) (g *glyph, err error) {
	if c >= firstBasic && c <= lastBasic {
		// Basic ASCII glyph case.
		g = &f.basicGlyphs[c-firstBasic]
		if g.sdf == nil {
			// Glyph not yet created. create it.
			gc, err := f.makeGlyph(c)
			if err != nil {
				return nil, err
			}
			*g = gc
		}
		return g, nil
	}
	// Unicode or other glyph.
	g, ok := f.otherGlyphs[c]
	if !ok {
		gc, err := f.makeGlyph(c)
		if err != nil {
			return nil, err
		}
		g = &gc
		f.otherGlyphs[c] = g
	}
	return g, nil
}

func (f *Font) scale() fixed.Int26_6 {
	units := f.sfn.UnitsPerEm()
	return fixed.Int26_6(units)
}

func (f *Font) rawbounds() ms2.Box {
	bb, _ := f.sfn.Bounds(&f.buf, f.scale(), font.HintingNone)
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
	bld := f.bld

	idx, err := f.sfn.GlyphIndex(&f.buf, char)
	if err != nil {
		return glyph{}, err
	}

	ppem := f.scale()
	segments, err := f.sfn.LoadGlyph(&f.buf, idx, ppem, nil)
	if err != nil {
		return glyph{}, err
	}

	scaleout := f.scaleout()
	tol := f.reltol

	// Split segments into contours (each MoveTo starts a new contour).
	contours := splitContours(segments)
	if len(contours) == 0 {
		return glyph{}, errors.New("glyph has no contours")
	}

	// Build first contour.
	shape, fill, err := segmentsToPolygon(bld, contours[0], tol, scaleout)
	if err != nil {
		return glyph{}, err
	}
	_ = fill // First contour fill direction is not necessarily an error.

	// Process remaining contours.
	for _, contour := range contours[1:] {
		sdf, fill, err := segmentsToPolygon(bld, contour, tol, scaleout)
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

// splitContours splits segments into separate contours. Each contour starts with a MoveTo.
func splitContours(segments sfnt.Segments) []sfnt.Segments {
	var contours []sfnt.Segments
	var current sfnt.Segments
	for _, seg := range segments {
		if seg.Op == sfnt.SegmentOpMoveTo && len(current) > 0 {
			contours = append(contours, current)
			current = nil
		}
		current = append(current, seg)
	}
	if len(current) > 0 {
		contours = append(contours, current)
	}
	return contours
}

var (
	quadBezier  = ms2.SplineBezierQuadratic()
	cubicBezier = ms2.SplineBezierCubic()
)

// segmentsToPolygon converts sfnt segments to a polygon.
// Returns the polygon SDF, whether it's a fill (positive winding), and any error.
func segmentsToPolygon(bld *gsdf.Builder, segments sfnt.Segments, tol, scale float32) (glbuild.Shader2D, bool, error) {
	var (
		poly       []ms2.Vec
		windingSum float32
		prev       ms2.Vec
		sampler    = ms2.Spline3Sampler{Tolerance: tol}
	)

	for _, seg := range segments {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			// Start of contour - note: sfnt Y axis increases downward, so we negate Y.
			prev = fixedToVec(seg.Args[0], scale)

		case sfnt.SegmentOpLineTo:
			p := fixedToVec(seg.Args[0], scale)
			poly = append(poly, prev)
			windingSum += (prev.X - p.X) * (prev.Y + p.Y)
			prev = p

		case sfnt.SegmentOpQuadTo:
			// Quadratic bezier: prev -> Args[0] (control) -> Args[1] (end)
			ctrl := fixedToVec(seg.Args[0], scale)
			end := fixedToVec(seg.Args[1], scale)
			sampler.Spline = quadBezier
			sampler.SetSplinePoints(prev, ctrl, end, ms2.Vec{})
			poly = append(poly, prev)
			poly = sampler.SampleBisect(poly, 4)
			windingSum += (prev.X - end.X) * (prev.Y + end.Y)
			prev = end

		case sfnt.SegmentOpCubeTo:
			// Cubic bezier: prev -> Args[0] (ctrl1) -> Args[1] (ctrl2) -> Args[2] (end)
			ctrl1 := fixedToVec(seg.Args[0], scale)
			ctrl2 := fixedToVec(seg.Args[1], scale)
			end := fixedToVec(seg.Args[2], scale)
			sampler.Spline = cubicBezier
			sampler.SetSplinePoints(prev, ctrl1, ctrl2, end)
			poly = append(poly, prev)
			poly = sampler.SampleBisect(poly, 4)
			windingSum += (prev.X - end.X) * (prev.Y + end.Y)
			prev = end
		}
	}

	return bld.NewPolygon(poly), windingSum > 0, bld.Err()
}

// fixedToVec converts a fixed.Point26_6 to ms2.Vec with scaling.
// Note: sfnt has Y increasing downward, so we negate Y to flip to standard math coordinates.
func fixedToVec(p fixed.Point26_6, scale float32) ms2.Vec {
	return ms2.Vec{
		X: float32(p.X) * scale,
		Y: -float32(p.Y) * scale, // Negate Y to flip coordinate system.
	}
}
