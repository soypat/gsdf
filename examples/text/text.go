package main

import (
	"fmt"
	"log"
	"runtime"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/textsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

const dim = 20
const filename = "text.png"

func init() {
	runtime.LockOSThread()
}

func scene(bld *gsdf.Builder) (glbuild.Shader2D, error) {
	var f textsdf.Font
	f.Configure(textsdf.FontConfig{
		RelativeGlyphTolerance: 0.001,
	})
	err := f.LoadTTFBytes(textsdf.ISO3098TTF())
	if err != nil {
		return nil, err
	}
	var poly ms2.PolygonBuilder
	poly.Nagon(7, 1)
	vecs, _ := poly.AppendVecs(nil)
	return bld.NewPolygon(vecs), bld.Err()
	return f.TextLine("Abc")
}

func main() {
	useGPU := true
	if useGPU {
		term, err := gleval.Init1x1GLFW()
		if err != nil {
			log.Fatal(err)
		}
		defer term()
	}
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
	err = gsdfaux.RenderPNGFile(filename, sdf2, 300, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("PNG file rendered to", filename)
}
