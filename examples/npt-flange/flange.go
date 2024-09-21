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

func scene() (glbuild.Shader3D, error) {
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

	pipe, _ := threads.Nut(threads.NutParams{
		Thread: npt,
		Style:  threads.NutCircular,
	})

	// Base plate which goes bolted to joint.
	flange, _ = gsdf.NewCylinder(flangeD/2, flangeH, flangeH/8)

	// Join threaded section with flange.
	flange = gsdf.Translate(flange, 0, 0, -tlen/2)
	union := gsdf.SmoothUnion(0.2, pipe, flange)

	// Make through-hole in flange bottom. Holes usually done at the end
	// to avoid smoothing effects covering up desired negative space.
	hole, _ := gsdf.NewCylinder(internalDiameter/2, 4*flangeH, 0)
	union = gsdf.Difference(union, hole)
	// Convert from imperial inches units to millimeter:
	union = gsdf.Scale(union, 25.4)
	return union, nil
}

func run() error {
	useGPU := flag.Bool("gpu", false, "Enable GPU rendering")
	flag.Parse()
	s, err := scene()
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

	err = gsdfaux.Render(s, gsdfaux.RenderConfig{
		STLOutput:    fpstl,
		VisualOutput: fpvis,
		Resolution:   s.Bounds().Diagonal() / 200,
		UseGPU:       *useGPU,
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
