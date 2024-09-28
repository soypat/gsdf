package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"

	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

const visualization = "bolt.glsl"
const stl = "bolt.stl"

func init() {
	runtime.LockOSThread() // For when using GPU this is required.
}

// scene generates the 3D object for rendering.
func scene() (glbuild.Shader3D, error) {
	const L, shank = 8, 3
	threader := threads.ISO{D: 3, P: 0.5, Ext: true}
	M3, err := threads.Bolt(threads.BoltParams{
		Thread:      threader,
		Style:       threads.NutHex,
		TotalLength: L + shank,
		ShankLength: shank,
	})
	if err != nil {
		return nil, err
	}
	M3, _ = gsdf.Rotate(M3, 2.5*math.Pi/2, ms3.Vec{X: 1, Z: 0.1})
	return M3, nil
}

func run() error {
	useGPU := flag.Bool("gpu", false, "Enable GPU usage")
	flag.Parse()
	sdf, err := scene()
	if err != nil {
		return err
	}
	fpstl, err := os.Create(stl)
	if err != nil {
		return err
	}
	defer fpstl.Close()
	fpvis, err := os.Create(visualization)
	if err != nil {
		return err
	}
	defer fpvis.Close()

	err = gsdfaux.RenderShader3D(sdf, gsdfaux.RenderConfig{
		STLOutput:    fpstl,
		VisualOutput: fpvis,
		Resolution:   sdf.Bounds().Diagonal() / 200,
		UseGPU:       *useGPU,
	})

	return err
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("bolt example done")
}
