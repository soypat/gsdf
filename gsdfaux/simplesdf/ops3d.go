package simplesdf

import (
	"errors"
	"fmt"
	"os"

	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

// Shader returns the underlying [glbuild.Shader3D] for use with the wider gsdf ecosystem.
func (s SDF3) Shader() glbuild.Shader3D { return s.s }

// K sets the pending smooth-blend radius used by the next boolean operation.
// The k value is consumed (reset to 0) after one boolean op.
// Transforms preserve k so it survives a chain of .Translate/.Rotate/etc.
func (s SDF3) K(k float64) SDF3 { s.k = k; return s }

// --- Boolean operations ---

// Union joins this shape with one or more others.
// Smooth union is used when either the receiver or an argument carries k > 0 (set via [SDF3.K]).
// k is consumed (reset to 0) in the returned shape.
func (s SDF3) Union(others ...SDF3) SDF3 {
	if len(others) == 0 {
		return s
	}
	// Check if any k is set across receiver or arguments.
	hasK := s.k > 0
	if !hasK {
		for _, o := range others {
			if o.k > 0 {
				hasK = true
				break
			}
		}
	}
	if hasK {
		result := s.s
		for _, o := range others {
			k := s.k
			if o.k > k {
				k = o.k
			}
			result = bld.SmoothUnion(float32(k), result, o.s)
		}
		return wrap3("SmoothUnion", result)
	}
	// Sharp batch union — bld.Union flattens nested unions.
	shaders := make([]glbuild.Shader3D, 1+len(others))
	shaders[0] = s.s
	for i, o := range others {
		shaders[i+1] = o.s
	}
	return wrap3("Union", bld.Union(shaders...))
}

// Diff subtracts b from s.
// Smooth difference is used when either the receiver or b carries k > 0.
// This matches fogleman's pattern: f -= tool.K(0.1) or f.K(0.1).Diff(tool).
func (s SDF3) Diff(b SDF3) SDF3 {
	k := s.k
	if b.k > k {
		k = b.k
	}
	if k > 0 {
		return wrap3("SmoothDiff", bld.SmoothDifference(float32(k), s.s, b.s))
	}
	return wrap3("Diff", bld.Difference(s.s, b.s))
}

// Intersect returns the region inside both s and b.
// Smooth intersection is used when either the receiver or b carries k > 0.
func (s SDF3) Intersect(b SDF3) SDF3 {
	k := s.k
	if b.k > k {
		k = b.k
	}
	if k > 0 {
		return wrap3("SmoothIntersect", bld.SmoothIntersect(float32(k), s.s, b.s))
	}
	return wrap3("Intersect", bld.Intersection(s.s, b.s))
}

// Xor returns the region inside s or b but not both (exclusive union).
func (s SDF3) Xor(b SDF3) SDF3 { return wrap3("Xor", bld.Xor(s.s, b.s)) }

// --- Transforms (preserve pending k) ---

// Translate moves the shape by (x, y, z).
func (s SDF3) Translate(x, y, z float64) SDF3 {
	return s.with("Translate", bld.Translate(s.s, float32(x), float32(y), float32(z)))
}

// Scale uniformly scales the shape by factor.
func (s SDF3) Scale(factor float64) SDF3 {
	return s.with("Scale", bld.Scale(s.s, float32(factor)))
}

// Rotate rotates the shape by radians around the axis (ax, ay, az).
func (s SDF3) Rotate(radians, ax, ay, az float64) SDF3 {
	return s.with("Rotate", bld.Rotate(s.s, float32(radians), ms3.Vec{X: float32(ax), Y: float32(ay), Z: float32(az)}))
}

// RotateX rotates the shape around the X axis.
func (s SDF3) RotateX(radians float64) SDF3 {
	return s.with("RotateX", bld.Rotate(s.s, float32(radians), ms3.Vec{X: 1}))
}

// RotateY rotates the shape around the Y axis.
func (s SDF3) RotateY(radians float64) SDF3 {
	return s.with("RotateY", bld.Rotate(s.s, float32(radians), ms3.Vec{Y: 1}))
}

// RotateZ rotates the shape around the Z axis.
func (s SDF3) RotateZ(radians float64) SDF3 {
	return s.with("RotateZ", bld.Rotate(s.s, float32(radians), ms3.Vec{Z: 1}))
}

// Mirror reflects the shape across the planes selected by x, y, z.
func (s SDF3) Mirror(x, y, z bool) SDF3 {
	return s.with("Mirror", bld.Symmetry(s.s, x, y, z))
}

// --- Shape modifiers (preserve pending k) ---

// Shell hollows out the shape leaving a shell of the given thickness.
func (s SDF3) Shell(thickness float64) SDF3 {
	return s.with("Shell", bld.Shell(s.s, float32(thickness)))
}

// Offset expands (positive delta) or contracts (negative delta) the shape's surface.
func (s SDF3) Offset(delta float64) SDF3 {
	return s.with("Offset", bld.Offset(s.s, float32(delta)))
}

// Elongate stretches the shape along each axis by the given amounts.
func (s SDF3) Elongate(x, y, z float64) SDF3 {
	return s.with("Elongate", bld.Elongate(s.s, float32(x), float32(y), float32(z)))
}

// Twist twists the shape around the Z axis with the given rate k (radians per unit length).
func (s SDF3) Twist(k float64) SDF3 {
	return s.with("Twist", bld.Twist(s.s, float32(k)))
}

// Array repeats the shape in a rectangular grid of nx×ny×nz copies with the given spacing.
func (s SDF3) Array(nx, ny, nz int, sx, sy, sz float64) SDF3 {
	return s.with("Array", bld.Array(s.s, float32(sx), float32(sy), float32(sz), nx, ny, nz))
}

// CircArray repeats the shape count times around a circle divided into circleDiv equal sectors.
func (s SDF3) CircArray(count, circleDiv int) SDF3 {
	return s.with("CircArray", bld.CircularArray(s.s, count, circleDiv))
}

// --- Output ---

// SaveSTL renders the shape to an STL file at filename.
// An optional [STLConfig] controls resolution and GPU usage; the zero value uses safe CPU defaults.
func (s SDF3) SaveSTL(filename string, cfg ...STLConfig) error {
	c := STLConfig{ResolutionDivisions: 1 << 9}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	res := c.Resolution
	if res == 0 {
		divs := c.ResolutionDivisions
		if divs == 0 {
			divs = 1 << 9
		}
		res = float64(s.s.Bounds().Diagonal()) / float64(divs)
	}
	fp, err := os.Create(filename)
	if err != nil {
		return makeErr("creating STL", err)
	}
	defer fp.Close()
	return withErr("rendering 3D sdf to STL", gsdfaux.RenderShader3D(s.s, gsdfaux.RenderConfig{
		STLOutput:     fp,
		Resolution:    float32(res),
		UseGPU:        c.UseGPU,
		EnableCaching: c.UseCache,
		ParallelCPU:   c.ParallelCPU,
	}))
}

func makeErr(msg string, err error) error {
	if err == nil {
		err = errors.New(msg)
	} else {
		err = fmt.Errorf("%s: %w", msg, err)
	}
	if panicMode {
		panic(err)
	}
	return err
}

func withErr(msg string, err error) error {
	if err == nil {
		return nil
	}
	err = fmt.Errorf("%s: %w", msg, err)
	if panicMode {
		panic(err)
	}
	return err
}
