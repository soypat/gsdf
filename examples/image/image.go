package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms1"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/glrender"
)

const size = 256
const dim = 20

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 2*size, size))
	renderer, err := glrender.NewImageRendererSDF2(4096, colorConversion)
	if err != nil {
		log.Fatal(err)
	}

	s, _ := gsdf.NewCircle(dim)
	poly, _ := gsdf.NewPolygon([]ms2.Vec{
		{X: dim, Y: 0},
		{X: 3 * dim, Y: dim},
		{X: 3 * dim, Y: -dim},
	})
	s = gsdf.Union2D(s, poly)
	fmt.Println(s.Bounds())
	sdf, err := gleval.NewCPUSDF2(s)
	if err != nil {
		log.Fatal(err)
	}
	err = renderer.Render(sdf, img, nil)
	if err != nil {
		log.Fatal(err)
	}
	fp, err := os.Create("circle.png")
	if err != nil {
		log.Fatal(err)
	}
	err = png.Encode(fp, img)
	if err != nil {
		log.Fatal(err)
	}
}

func colorConversion(d float32) color.Color {
	d /= dim * 2
	var one = ms3.Vec{1, 1, 1}
	var c ms3.Vec
	if d > 0 {
		c = ms3.Vec{0.9, 0.6, 0.3}
	} else {
		c = ms3.Vec{0.65, 0.85, 1.0}
	}
	c = ms3.Scale(1-math32.Exp(-6*math32.Abs(d)), c)
	c = ms3.Scale(0.8+0.2*math32.Cos(150*d), c)
	max := 1 - ms1.SmoothStep(0, 0.01, math32.Abs(d))
	c = ms3.InterpElem(c, one, ms3.Vec{max, max, max})
	return color.RGBA{
		R: uint8(c.X * 255),
		G: uint8(c.Y * 255),
		B: uint8(c.Z * 255),
		A: 255,
	}
}
