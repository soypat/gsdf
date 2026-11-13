package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"time"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/textsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gpusdf"
	"github.com/soypat/gsdf/gsdfaux"
)

const filename = "text.png"

func scene(bld *gsdf.Builder) (glbuild.Shader2D, error) {
	var f textsdf.Font
	f.Configure(textsdf.FontConfig{
		RelativeGlyphTolerance: 0.001,
	})
	err := f.LoadTTFBytes(textsdf.ISO3098TTF())
	if err != nil {
		return nil, err
	}
	return f.TextLine("Abc123~")
}

func main() {
	useGPU := false
	flag.BoolVar(&useGPU, "gpu", useGPU, "Enable GPU usage")
	flag.Parse()
	var dev gpusdf.Device
	if useGPU {
		err := dev.Configure(gpusdf.DeviceConfig{})
		if err != nil {
			log.Fatal(err)
		}
		defer dev.Release()
	}
	var bld gsdf.Builder
	s, err := scene(&bld)
	if err != nil {
		log.Fatal(err)
	}
	var sdf2 gleval.SDF2
	if useGPU {
		sdf2, err = gsdfaux.MakeGPUSDF2(&dev, s)
	} else {
		sdf2, err = gleval.NewCPUSDF2(s)
	}
	if err != nil {
		log.Fatal(err)
	}
	if !useGPU {
		fmt.Println("GPU usage not enabled (-gpu flag). Enable for faster rendering")
	}

	charHeight := sdf2.Bounds().Size().Y
	edgeAliasing := charHeight / 1000
	conversion := gsdfaux.ColorConversionLinearGradient(edgeAliasing, color.Black, color.White)
	start := time.Now()
	err = gsdfaux.RenderPNGFile(filename, sdf2, 300, conversion)
	if err != nil {
		log.Fatal(err)
	}
	_ = conversion
	fmt.Println("PNG file rendered to", filename, "in", time.Since(start))
}
