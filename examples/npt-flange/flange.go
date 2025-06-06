package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

const visualization = "nptflange.glsl"
const stl = "nptflange.stl"

func init() {
	runtime.LockOSThread() // For when using GPU this is required.
}

func scene(bld *gsdf.Builder) (glbuild.Shader3D, error) {
	const (
		tlen             = 18. / 25.4
		internalDiameter = 1.5 / 2.
		flangeH          = 7. / 25.4
		flangeD          = 60. / 25.4
	)
	var (
		npt    threads.NPT
		flange glbuild.Shader3D
		err    error
	)
	err = npt.SetFromNominal(1.0 / 2.0)
	if err != nil {
		return nil, err
	}

	pipe, _ := threads.Nut(bld, threads.NutParams{
		Thread: npt,
		Style:  threads.NutCircular,
	})

	// Base plate which goes bolted to joint.
	flange = bld.NewCylinder(flangeD/2, flangeH, flangeH/8)

	// Join threaded section with flange.
	flange = bld.Translate(flange, 0, 0, -tlen/2)
	union := bld.SmoothUnion(0.2, pipe, flange)

	// Make through-hole in flange bottom. Holes usually done at the end
	// to avoid smoothing effects covering up desired negative space.
	hole := bld.NewCylinder(internalDiameter/2, 4*flangeH, 0)
	union = bld.Difference(union, hole)
	// Convert from imperial inches units to millimeter:
	union = bld.Scale(union, 25.4)
	return union, bld.Err()
}

func run() error {
	var (
		useGPU     bool
		resolution float64
		flagResDiv uint
	)
	flag.BoolVar(&useGPU, "gpu", false, "enable GPU usage")
	flag.Float64Var(&resolution, "res", 0, "Set resolution in shape units. Useful for setting the minimum level of detail to a fixed amount for final result. If not set resdiv used [mm/in]")
	flag.UintVar(&flagResDiv, "resdiv", 200, "Set resolution in bounding box diagonal divisions. Useful for prototyping when constant speed of rendering is desired.")
	flag.Parse()
	var bld gsdf.Builder
	object, err := scene(&bld)
	if err != nil {
		return err
	}
	if resolution == 0 {
		resolution = float64(object.Bounds().Diagonal()) / float64(flagResDiv)
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

	err = gsdfaux.RenderShader3D(object, gsdfaux.RenderConfig{
		STLOutput:    fpstl,
		VisualOutput: fpvis,
		Resolution:   float32(resolution),
		UseGPU:       useGPU,
	})
	return err
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("finished npt-flange example")
}
