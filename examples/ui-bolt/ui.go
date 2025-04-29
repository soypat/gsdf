package main

import (
	"log"
	"math"
	"runtime"

	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

func init() {
	runtime.LockOSThread()
}

// scene generates the 3D object for rendering.
func scene(bld *gsdf.Builder) (glbuild.Shader3D, error) {
	const L, shank = 8, 3
	threader := threads.ISO{D: 3, P: 0.5, Ext: true}
	M3, err := threads.Bolt(bld, threads.BoltParams{
		Thread:      threader,
		Style:       threads.NutHex,
		TotalLength: L + shank,
		ShankLength: shank,
	})
	if err != nil {
		return nil, err
	}
	M3 = bld.Rotate(M3, 2.5*math.Pi/2, ms3.Vec{X: 1, Z: 0.1})
	return M3, bld.Err()
}

func main() {
	var bld gsdf.Builder
	shape, err := scene(&bld)
	shape = bld.Scale(shape, 0.3)
	if err != nil {
		log.Fatal("creating scene:", err)
	}
	err = gsdfaux.UI(shape, gsdfaux.UIConfig{
		Width:  800,
		Height: 600,
	})
	if err != nil {
		log.Fatal("UI:", err)
	}
}
