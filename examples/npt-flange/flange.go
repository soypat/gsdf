package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/glrender"
)

func init() {
	flag.BoolVar(&useGPU, "gpu", useGPU, "Enable GPU usage")
	flag.Parse()
	if useGPU {
		fmt.Println("enable GPU usage")
		runtime.LockOSThread() // For when using GPU this is required.
	}
}

var useGPU = false

const visualization = "nptflange.glsl"

func main() {
	if useGPU {
		terminate, err := gleval.Init1x1GLFW()
		if err != nil {
			log.Fatal("FAIL to start GLFW", err.Error())
		}
		defer terminate()
	}
	sdf, err := scene()
	if err != nil {
		fmt.Println("error making scene:", err)
		os.Exit(1)
	}
	const resDiv = 200
	const evaluationBufferSize = 1024 * 8
	resolution := sdf.Bounds().Size().Max() / resDiv
	renderer, err := glrender.NewOctreeRenderer(sdf, resolution, evaluationBufferSize)
	if err != nil {
		fmt.Println("error creating renderer:", err)
		os.Exit(1)
	}
	triangles, err := glrender.RenderAll(renderer)
	if err != nil {
		fmt.Println("error rendering triangles:", err)
		os.Exit(1)
	}
	fp, err := os.Create("nptflange.stl")
	if err != nil {
		fmt.Println("error creating file:", err)
		os.Exit(1)
	}
	defer fp.Close()
	_, err = glrender.WriteBinarySTL(fp, triangles)
	if err != nil {
		fmt.Println("error creating file:", err)
		os.Exit(1)
	}
}

func scene() (gleval.SDF3, error) {
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

	pipe, _ := threads.Nut(threads.NutParms{
		Thread: npt,
		Style:  threads.NutCircular,
	})

	// Base plate which goes bolted to joint.
	flange, _ = gsdf.NewCylinder(flangeD/2, flangeH, flangeH/8)

	// Join threaded section with flange.
	flange = gsdf.Translate(flange, 0, 0, -tlen/2)
	union := gsdf.SmoothUnion(pipe, flange, 0.2)

	// Make through-hole in flange bottom. Holes usually done at the end
	// to avoid smoothing effects covering up desired negative space.
	hole, _ := gsdf.NewCylinder(internalDiameter/2, 4*flangeH, 0)
	union = gsdf.Difference(union, hole)
	// Convert from imperial inches units to millimeter:
	union = gsdf.Scale(union, 25.4)
	return makeSDF(union)
}

func makeSDF(s glbuild.Shader3D) (gleval.SDF3, error) {
	err := glbuild.RewriteNames3D(&s, 32) // Shorten names to not crash GL tokenizer.
	if err != nil {
		return nil, err
	}
	if visualization != "" {
		const sceneSize = 0.5
		// We include the bounding box in the visualization.
		bb := s.Bounds()
		envelope, err := gsdf.NewBoundsBoxFrame(bb)
		if err != nil {
			return nil, err
		}
		visual := gsdf.Union(s, envelope)
		// Scale size and translate to center so visualization is in camera range.
		center := bb.Center()
		visual = gsdf.Translate(visual, center.X, center.Y, center.Z)
		visual = gsdf.Scale(visual, sceneSize/bb.Size().Max())
		source := new(bytes.Buffer)
		_, err = glbuild.NewDefaultProgrammer().WriteFragVisualizerSDF3(source, visual)
		if err != nil {
			return nil, err
		}
		err = os.WriteFile(visualization, source.Bytes(), 0666)
		if err != nil {
			return nil, err
		}
	}
	if useGPU {
		source := new(bytes.Buffer)
		_, err := glbuild.NewDefaultProgrammer().WriteComputeSDF3(source, s)
		if err != nil {
			return nil, err
		}
		return gleval.NewComputeGPUSDF3(source, s.Bounds())
	}
	return gleval.NewCPUSDF3(s)
}
