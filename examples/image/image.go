package main

import (
	"fmt"
	"log"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

const dim = 20
const filename = "circle.png"

func scene() (glbuild.Shader2D, error) {
	s, err := gsdf.NewCircle(dim)
	if err != nil {
		return nil, err
	}
	poly, err := gsdf.NewPolygon([]ms2.Vec{
		{X: dim, Y: 0},
		{X: 3 * dim, Y: dim},
		{X: 3 * dim, Y: -dim},
	})
	if err != nil {
		return nil, err
	}
	s = gsdf.Union2D(s, poly)
	return s, nil
}

func main() {
	useGPU := false
	s, err := scene()
	if err != nil {
		log.Fatal(err)
	}
	err = gsdfaux.RenderPNGFile(filename, s, 1080, useGPU, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("PNG file rendered")
}
