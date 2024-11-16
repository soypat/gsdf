package main

import (
	"log"
	"runtime"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

func scene2d(bld *gsdf.Builder) (glbuild.Shader2D, error) {
	circle := bld.NewCircle(1)
	hex := bld.NewHexagon(1)
	circle = bld.Translate2D(circle, 1, 1)
	shape := bld.Union2D(circle, hex)
	shape = bld.Offset2D(shape, .2)
	shape = bld.Annulus(shape, .3)

	shape = bld.Translate2D(shape, 3, 0)
	shape = bld.CircularArray2D(shape, 12, 12)
	return shape, nil
}

// scene generates the 3D object for rendering.
func scene(bld *gsdf.Builder) (glbuild.Shader3D, error) {
	mandala, _ := scene2d(bld)
	shape := bld.Extrude(mandala, 1)
	shape = bld.Offset(shape, -.1) // Negative offset does rounding.
	return shape, nil
}

func init() {
	runtime.LockOSThread()
}

func main() {
	var bld gsdf.Builder
	shape, err := scene(&bld)
	shape = bld.Scale(shape, 0.3)
	if err != nil {
		log.Fatal("creating scene:", err)
	}
	err = gsdfaux.UI(shape, gsdfaux.UIConfig{
		Width:  1000,
		Height: 600,
	})
	if err != nil {
		log.Fatal("UI:", err)
	}
}
