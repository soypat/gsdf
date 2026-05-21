package simplesdf

import "github.com/soypat/gsdf/glbuild"

// Shader returns the underlying [glbuild.Shader2D] for use with the wider gsdf ecosystem.
func (s SDF2) Shader() glbuild.Shader2D { return s.s }

// K sets the pending smooth-blend radius.
// Note: the gsdf library has no smooth 2D boolean operations, so k is carried through
// transforms but has no effect on 2D boolean ops.
func (s SDF2) K(k float64) SDF2 { s.k = k; return s }

// --- Boolean operations ---

// Union joins this shape with one or more others.
func (s SDF2) Union(others ...SDF2) SDF2 {
	if len(others) == 0 {
		return s
	}
	shaders := make([]glbuild.Shader2D, 1+len(others))
	shaders[0] = s.s
	for i, o := range others {
		shaders[i+1] = o.s
	}
	return wrap2(bld.Union2D(shaders...))
}

// Diff subtracts b from s.
func (s SDF2) Diff(b SDF2) SDF2 { return wrap2(bld.Difference2D(s.s, b.s)) }

// Intersect returns the region inside both s and b.
func (s SDF2) Intersect(b SDF2) SDF2 { return wrap2(bld.Intersection2D(s.s, b.s)) }

// Xor returns the region inside s or b but not both.
func (s SDF2) Xor(b SDF2) SDF2 { return wrap2(bld.Xor2D(s.s, b.s)) }

// --- Transforms (preserve pending k) ---

// Translate moves the shape by (x, y).
func (s SDF2) Translate(x, y float64) SDF2 {
	return s.with(bld.Translate2D(s.s, float32(x), float32(y)))
}

// Scale uniformly scales the shape by factor.
func (s SDF2) Scale(factor float64) SDF2 { return s.with(bld.Scale2D(s.s, float32(factor))) }

// Rotate rotates the shape by radians around the origin.
func (s SDF2) Rotate(radians float64) SDF2 { return s.with(bld.Rotate2D(s.s, float32(radians))) }

// Mirror reflects the shape across the axes selected by x, y.
func (s SDF2) Mirror(x, y bool) SDF2 { return s.with(bld.Symmetry2D(s.s, x, y)) }

// --- Shape modifiers (preserve pending k) ---

// Offset expands (positive delta) or contracts (negative delta) the shape's boundary.
func (s SDF2) Offset(delta float64) SDF2 { return s.with(bld.Offset2D(s.s, float32(delta))) }

// Elongate stretches the shape along each axis by the given amounts.
func (s SDF2) Elongate(x, y float64) SDF2 {
	return s.with(bld.Elongate2D(s.s, float32(x), float32(y)))
}

// Array repeats the shape in a rectangular grid of nx×ny copies with the given spacing.
func (s SDF2) Array(nx, ny int, sx, sy float64) SDF2 {
	return s.with(bld.Array2D(s.s, float32(sx), float32(sy), nx, ny))
}

// CircArray repeats the shape count times around a circle divided into circleDiv equal sectors.
func (s SDF2) CircArray(count, circleDiv int) SDF2 {
	return s.with(bld.CircularArray2D(s.s, count, circleDiv))
}

// --- 2D → 3D conversions ---

// Extrude extrudes the 2D shape into a 3D solid of height h along Z.
func (s SDF2) Extrude(h float64) SDF3 { return wrap3(bld.Extrude(s.s, float32(h))) }

// Revolve revolves the 2D shape around the Y axis, offset from the axis by offset units.
func (s SDF2) Revolve(offset float64) SDF3 { return wrap3(bld.Revolve(s.s, float32(offset))) }
