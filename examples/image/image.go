package main

import (
	"fmt"
	"log"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

const dim = 20
const filename = "image-example.png"

func scene(bld *gsdf.Builder) (glbuild.Shader2D, error) {
	s := bld.NewCircle(dim)
	poly := bld.NewPolygon([]ms2.Vec{
		{X: dim, Y: 0},
		{X: 3 * dim, Y: dim},
		{X: 3 * dim, Y: -dim},
	})
	s = bld.Union2D(s, poly)
	return s, bld.Err()
}

func main() {
	useGPU := false
	var bld gsdf.Builder
	s, err := scene(&bld)
	if err != nil {
		log.Fatal(err)
	}
	var sdf2 gleval.SDF2
	if useGPU {
		sdf2, err = gsdfaux.MakeGPUSDF2(s)
	} else {
		sdf2, err = gleval.NewCPUSDF2(s)
	}
	if err != nil {
		log.Fatal(err)
	}
	err = gsdfaux.RenderPNGFile(filename, sdf2, 1080, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("PNG file rendered to", filename)
}
