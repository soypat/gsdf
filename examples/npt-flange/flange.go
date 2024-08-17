package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/soypat/glgl/v4.6-core/glgl"
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
		_, terminate, err := glgl.InitWithCurrentWindow33(glgl.WindowConfig{
			Title:   "compute",
			Version: [2]int{4, 6},
			Width:   1,
			Height:  1,
		})
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
	const evaluationBufferSize = 1024
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
		// visualization is the name of the file with a GLSL
		// generated visualization of the SDF which can be visualized in https://www.shadertoy.com/
		// or using VSCode's ShaderToy extension. If visualization=="" then no file is generated.
		// thread length
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

	pipe, err := threads.Nut(threads.NutParms{
		Thread: threads.ISO{D: npt.D, P: 1.0 / npt.TPI},
		Style:  threads.NutHex,
	})
	if err != nil {
		return nil, err
	}
	flange, err = gsdf.NewCylinder(flangeD/2, flangeH, flangeH/8)
	if err != nil {
		return nil, err
	}
	flange = gsdf.Translate(flange, 0, 0, -tlen/2)
	flange = gsdf.SmoothUnion(pipe, flange, 0.2)
	hole, err := gsdf.NewCylinder(internalDiameter/2, 4*flangeH, 0)
	if err != nil {
		return nil, err
	}
	// return makeSDF(hole)
	flange = gsdf.Difference(flange, hole) // Make through-hole in flange bottom
	flange = gsdf.Scale(flange, 25.4)      // convert to millimeters
	return makeSDF(flange)
}

func makeSDF(s glbuild.Shader3D) (gleval.SDF3, error) {
	if visualization != "" {
		const sceneSize = 0.5
		// We include the bounding box in the visualization.
		bb := s.Bounds()
		envelope, err := gsdf.NewBoundsBoxFrame(bb)
		if err != nil {
			return nil, err
		}
		s = gsdf.Union(s, envelope)
		// Scale size and translate to center so visualization is in camera range.
		center := bb.Center()
		s = gsdf.Translate(s, center.X, center.Y, center.Z)
		s = gsdf.Scale(s, sceneSize/bb.Size().Max())
		source := new(bytes.Buffer)
		_, err = glbuild.NewDefaultProgrammer().WriteFragVisualizerSDF3(source, s)
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
