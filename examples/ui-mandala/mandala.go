package main

import (
	"log"
	"runtime"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

func scene2d() (glbuild.Shader2D, error) {
	circle, _ := gsdf.NewCircle(1)
	hex, _ := gsdf.NewHexagon(1)
	circle = gsdf.Translate2D(circle, 1, 1)
	shape := gsdf.Union2D(circle, hex)
	shape = gsdf.Offset2D(shape, .2)
	shape, _ = gsdf.Annulus(shape, .3)

	shape = gsdf.Translate2D(shape, 3, 0)
	shape, _ = gsdf.CircularArray2D(shape, 12, 12)
	return shape, nil
}

// scene generates the 3D object for rendering.
func scene() (glbuild.Shader3D, error) {
	mandala, _ := scene2d()
	shape, _ := gsdf.Extrude(mandala, 1)
	shape = gsdf.Offset(shape, -.1) // Negative offset does rounding.
	return shape, nil
}

func init() {
	runtime.LockOSThread()
}

func main() {
	shape, err := scene()
	shape = gsdf.Scale(shape, 0.3)
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
