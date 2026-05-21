package main

import (
	"math"
	"runtime"

	. "github.com/soypat/gsdf/gsdfaux/simplesdf"
)

func init() { runtime.LockOSThread() } // Required if using GPU to render shapes or using UI.

func main() {
	// main body
	f := Cylinder(1, 5, 0.1)

	// knurling
	x := Box(1, 1, 4, 0).RotateZ(math.Pi / 4)
	x = x.Translate(1.6, 0, 0)              // radial placement (fogleman circular_array offset)
	x = x.CircArray(24, 24)                 // 24 instances evenly around the circle
	x = x.Twist(0.75).Union(x.Twist(-0.75)) // diamond pattern via mirrored twist
	f = f.Diff(x.K(0.1))

	// central hole
	f = f.Diff(Cylinder(0.5, 7, 0).K(0.1))

	// vent holes
	c := Cylinder(0.25, 3, 0).RotateY(math.Pi / 2) // orient along X
	f = f.Diff(c.Translate(0, 0, -2.5).K(0.1))
	f = f.Diff(c.Translate(0, 0, 2.5).K(0.1))

	f.Save("knurling.stl", STLConfig{ResolutionDivisions: 200})
}
