package simplesdf

import (
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

// SDF3 is an immutable, chainable 3D signed-distance-function value.
// Every method returns a new SDF3; the receiver is never modified.
// Boolean operations consume the pending smooth-blend radius k set via [SDF3.K].
type SDF3 struct {
	s glbuild.Shader3D
	k float64 // pending smooth-blend radius; 0 = sharp boolean ops
}

// SDF2 is an immutable, chainable 2D signed-distance-function value.
type SDF2 struct {
	s glbuild.Shader2D
	k float64
}

// STLConfig controls mesh rendering output for [SDF3.SaveSTL].
// A zero-value STLConfig is valid and uses safe CPU defaults.
type STLConfig struct {
	UseGPU              bool    // enable GPU rendering (requires OpenGL context)
	Resolution          float64 // minimum triangle size; overrides ResolutionDivisions when > 0
	ResolutionDivisions uint    // bounding-box subdivisions used when Resolution == 0 (default 1<<9)
}

var (
	bld       gsdf.Builder
	panicMode = true
)

// SetPanicMode controls whether invalid shape arguments panic (true, default) or
// accumulate silently and are retrievable via [Err].
func SetPanicMode(enabled bool) {
	panicMode = enabled
	flags := bld.Flags()
	if enabled {
		flags &^= gsdf.FlagNoDimensionPanic
	} else {
		flags |= gsdf.FlagNoDimensionPanic
	}
	bld.SetFlags(flags)
}

// Err returns any errors accumulated since the last [ClearErrors] call.
// Only meaningful when panic mode is disabled via [SetPanicMode].
func Err() error { return bld.Err() }

// ClearErrors resets the accumulated error state.
func ClearErrors() { bld.ClearErrors() }

// wrap3 wraps a Shader3D into SDF3 with k reset to 0 (boolean ops consume k).
func wrap3(msg string, s glbuild.Shader3D) SDF3 {
	withErr(msg, bld.Err())
	return SDF3{s: s}
}

// wrap2 wraps a Shader2D into SDF2 with k reset to 0.
func wrap2(msg string, s glbuild.Shader2D) SDF2 {
	withErr(msg, bld.Err())
	return SDF2{s: s}
}

// with returns a copy of s with a new shader but the same k value.
// Used by transforms so that a pending K survives translate/rotate/etc.
func (s SDF3) with(msg string, shader glbuild.Shader3D) SDF3 {
	withErr(msg, bld.Err())
	return SDF3{s: shader, k: s.k}
}
func (s SDF2) with(msg string, shader glbuild.Shader2D) SDF2 {
	withErr(msg, bld.Err())
	return SDF2{s: shader, k: s.k}
}
