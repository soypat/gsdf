package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"runtime"
	"time"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms1"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/textsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

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
	return f.TextLine("Abc123~")
}

func main() {
	useGPU := false
	flag.BoolVar(&useGPU, "gpu", useGPU, "Enable GPU usage")
	flag.Parse()
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
	if !useGPU {
		fmt.Println("GPU usage not enabled (-gpu flag). Enable for faster rendering")
	}

	charHeight := sdf2.Bounds().Size().Y
	edgeAliasing := charHeight / 1000
	start := time.Now()
	err = gsdfaux.RenderPNGFile(filename, sdf2, 300, blackAndWhite(edgeAliasing))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("PNG file rendered to", filename, "in", time.Since(start))
}

func blackAndWhite(edgeSmooth float32) func(d float32) color.Color {
	if edgeSmooth <= 0 {
		return blackAndWhiteNoSmoothing
	}
	return func(d float32) color.Color {
		// Smoothstep anti-aliasing near the edge
		blend := 0.5 + 0.5*math32.Tanh(d/edgeSmooth)
		// Clamp blend to [0, 1] for valid grayscale values
		blend = ms1.Clamp(blend, 0, 1)
		// Convert blend to grayscale
		grayValue := uint8(blend * 255)
		return color.Gray{Y: grayValue}
	}
}

func blackAndWhiteNoSmoothing(d float32) color.Color {
	if d < 0 {
		return color.Black
	}
	return color.White
}
