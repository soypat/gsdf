package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/glrender"
)

const visualization = "bolt.glsl"
const stl = "bolt.stl"

var useGPU = false

func init() {
	flag.BoolVar(&useGPU, "gpu", useGPU, "Enable GPU usage")
	flag.Parse()
	if useGPU {
		fmt.Println("enabled GPU usage")
		runtime.LockOSThread() // For when using GPU this is required.
	}
}

// scene generates the 3D object for rendering.
func scene() (gleval.SDF3, error) {
	const L, shank = 5, 3
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
	return makeSDF(M3)
}

func main() {
	if useGPU {
		terminate, err := gleval.Init1x1GLFW()
		if err != nil {
			log.Fatal("failed to start GLFW", err.Error())
		}
		defer terminate()
	}
	sceneStart := time.Now()
	sdf, err := scene()
	if err != nil {
		fmt.Println("error making scene:", err)
		os.Exit(1)
	}
	elapsedScene := time.Since(sceneStart)
	const resDiv = 200
	const evaluationBufferSize = 1024 * 8
	resolution := sdf.Bounds().Size().Max() / resDiv
	renderer, err := glrender.NewOctreeRenderer(sdf, resolution, evaluationBufferSize)
	if err != nil {
		fmt.Println("error creating renderer:", err)
		os.Exit(1)
	}
	start := time.Now()
	triangles, err := glrender.RenderAll(renderer)
	if err != nil {
		fmt.Println("error rendering triangles:", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)
	evals := sdf.(interface{ Evaluations() uint64 }).Evaluations()

	fp, err := os.Create(stl)
	if err != nil {
		fmt.Println("error creating file:", err)
		os.Exit(1)
	}
	defer fp.Close()
	start = time.Now()
	w := bufio.NewWriter(fp)
	_, err = glrender.WriteBinarySTL(w, triangles)
	if err != nil {
		fmt.Println("error writing triangles to file:", err)
		os.Exit(1)
	}
	w.Flush()
	fmt.Println("SDF created in ", elapsedScene, "evaluated sdf", evals, "times, rendered", len(triangles), "triangles in", elapsed, "wrote file in", time.Since(start))
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
