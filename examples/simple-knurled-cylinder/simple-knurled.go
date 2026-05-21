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

	f.SaveSTL("knurling.stl", STLConfig{UseGPU: false, Resolution: 0.01})
}

/* Equivalent Python program using github.com/fogleman/sdf library.

from sdf import *

# main body
f = rounded_cylinder(1, 0.1, 5)

# knurling
x = box((1, 1, 4)).rotate(pi / 4)
x = x.circular_array(24, 1.6)
x = x.twist(0.75) | x.twist(-0.75)
f -= x.k(0.1)

# central hole
f -= cylinder(0.5).k(0.1)

# vent holes
c = cylinder(0.25).orient(X)
f -= c.translate(Z * -2.5).k(0.1)
f -= c.translate(Z * 2.5).k(0.1)

f.save('knurling.stl', step=.01)
*/
