package gsdf

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
)

const (
	// For an equilateral triangle of side length L the length of bisector is L multiplied this number which is sqrt(1-0.25).
	tribisect = 0.8660254037844386467637231707529361834714026269051903140279034897
	sqrt2d2   = math32.Sqrt2 / 2
	sqrt3     = 1.7320508075688772935274463415058723669428052538103806280558069794
	largenum  = 1e20
	// epstol is used to check for badly conditioned denominators
	// such as lengths used for normalization or transformation matrix determinants.
	epstol = 6e-7
)

// Builder wraps all SDF primitive and operation logic generation.
// Provides error handling strategies with panics or error accumulation during shape generation.
type Builder struct {
	NoDimensionPanic bool
	accumErrs        []error
}

func (bld *Builder) Err() error {
	if len(bld.accumErrs) == 0 {
		return nil
	}
	return errors.Join(bld.accumErrs...)
}

func (bld *Builder) shapeErrorf(msg string, args ...any) {
	if !bld.NoDimensionPanic {
		panic(fmt.Sprintf(msg, args...))
	}
	// bld.stacks = append(bld.stacks, string(debug.Stack()))
	bld.accumErrs = append(bld.accumErrs, fmt.Errorf(msg, args...))
}

func (*Builder) nilsdf(msg string) {
	panic("nil SDF argument: " + msg)
}

// These interfaces are implemented by all SDF interfaces such as SDF3/2 and Shader3D/2D.
// Using these instead of `any` Aids in catching mistakes at compile time such as passing a Shader3D instead of Shader2D as an argument.
type (
	bounder2 = interface{ Bounds() ms2.Box }
	bounder3 = interface{ Bounds() ms3.Box }
)

func minf(a, b float32) float32 {
	return math32.Min(a, b)
}
func hypotf(a, b float32) float32 {
	return math32.Hypot(a, b)
}

func signf(a float32) float32 {
	if a == 0 {
		return 0
	}
	return math32.Copysign(1, a)
}

func clampf(v, Min, Max float32) float32 {
	// return ms3.Clamp(v, Min, Max)
	if v < Min {
		return Min
	} else if v > Max {
		return Max
	}
	return v
}

func mixf(x, y, a float32) float32 {
	return x*(1-a) + y*a
}

func maxf(a, b float32) float32 {
	return math32.Max(a, b)
}

func absf(a float32) float32 {
	return math32.Abs(a)
}

func hashvec2(vecs ...ms2.Vec) float32 {
	var hashA float32 = 0.0
	var hashB float32 = 1.0
	for _, v := range vecs {
		hashA, hashB = hashAdd(hashA, hashB, v.X)
		hashA, hashB = hashAdd(hashA, hashB, v.Y)
	}
	return hashfint(hashA + hashB)
}

func hash2vec2(vecs ...[2]ms2.Vec) float32 {
	var hashA float32 = 0.0
	var hashB float32 = 1.0
	for _, v := range vecs {
		hashA, hashB = hashAdd(hashA, hashB, v[0].X)
		hashA, hashB = hashAdd(hashA, hashB, v[0].Y)
		hashA, hashB = hashAdd(hashA, hashB, v[1].X)
		hashA, hashB = hashAdd(hashA, hashB, v[1].Y)
	}
	return hashfint(hashA + hashB)
}

func hashvec3(vecs ...ms3.Vec) float32 {
	var hashA float32 = 0.0
	var hashB float32 = 1.0
	for _, v := range vecs {
		hashA, hashB = hashAdd(hashA, hashB, v.X)
		hashA, hashB = hashAdd(hashA, hashB, v.Y)
		hashA, hashB = hashAdd(hashA, hashB, v.Z)
	}
	return hashfint(hashA + hashB)
}

func hashf(values []float32) float32 {
	var hashA float32 = 0.0
	var hashB float32 = 1.0
	for _, num := range values {
		hashA, hashB = hashAdd(hashA, hashB, num)
	}
	return hashfint(hashA + hashB)
}

func hashAdd(a, b, num float32) (aNew, bNew float32) {
	const prime = 31.0
	a += num
	b *= (prime + num)
	a = hashfint(a)
	b = hashfint(b)
	return a, b
}

func hashfint(f float32) float32 {
	return float32(int(f*1000000)%1000000) / 1000000 // Keep within [0.0, 1.0)
}
