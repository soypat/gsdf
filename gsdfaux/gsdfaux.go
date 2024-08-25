package gsdfaux

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"time"

	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/glrender"
)

type RenderConfig struct {
	STLOutput    io.Writer
	VisualOutput io.Writer
	Resolution   float32
	UseGPU       bool
	Silent       bool
}

// Render is an auxiliary function to aid users in getting setup in using gsdf quickly.
// Ideally users should implement their own rendering functions since applications may vary widely.
func Render(s glbuild.Shader3D, cfg RenderConfig) (err error) {
	if cfg.STLOutput == nil && cfg.VisualOutput == nil {
		return errors.New("Render requires output parameter in config")
	}
	log := func(args ...any) {
		if !cfg.Silent {
			fmt.Println(args...)
		}
	}
	err = glbuild.ShortenNames3D(&s, 6)
	if err != nil {
		return fmt.Errorf("shortening shader names: %s", err)
	}

	bb := s.Bounds()
	var sdf gleval.SDF3
	watch := stopwatch()
	if cfg.UseGPU {
		log("using GPU\tᵍᵒᵗᵗᵃ ᵍᵒ ᶠᵃˢᵗ")
		{
			terminate, err := gleval.Init1x1GLFW()
			if err != nil {
				return err
			}
			defer terminate()
		}
		source := new(bytes.Buffer)
		_, err = glbuild.NewDefaultProgrammer().WriteComputeSDF3(source, s)
		if err != nil {
			return err
		}
		sdf, err = gleval.NewComputeGPUSDF3(source, bb)
	} else {
		log("using CPU")
		sdf, err = gleval.NewCPUSDF3(s)
	}

	if err != nil || sdf == nil {
		return fmt.Errorf("instantiating SDF: %s", err)
	}

	log("instantiating evaluation SDF took", watch())
	const size = 1 << 12
	renderer, err := glrender.NewOctreeRenderer(sdf, cfg.Resolution, size)
	if err != nil {
		return err
	}

	if cfg.VisualOutput != nil {
		watch = stopwatch()
		const sceneSize = 1.4
		// We include the bounding box in the visualization.
		bb := s.Bounds()
		envelope, err := gsdf.NewBoundsBoxFrame(bb.Add(ms3.Vec{100, 100, 100}))
		if err != nil {
			return err
		}
		visual := gsdf.Union(s, envelope)
		// Scale size and translate to center so visualization is in camera range.
		center := bb.Center()
		sz := bb.Size()
		visual = gsdf.Translate(visual, center.X, center.Y, center.Z-sz.Z)
		visual = gsdf.Scale(visual, sceneSize/bb.Diagonal())
		_, err = glbuild.NewDefaultProgrammer().WriteFragVisualizerSDF3(cfg.VisualOutput, visual)
		if err != nil {
			return fmt.Errorf("writing visual GLSL: %s", err)
		}
		filename := "GLSL visualization"
		if fp, ok := cfg.VisualOutput.(*os.File); ok {
			filename = fp.Name()
		}
		log("wrote", filename, "in", watch())
	}

	if cfg.STLOutput != nil {
		watch = stopwatch()
		triangles, err := glrender.RenderAll(renderer)
		if err != nil {
			return fmt.Errorf("rendering triangles: %s", err)
		}
		e := sdf.(interface{ Evaluations() uint64 })
		omitted := 8 * renderer.TotalPruned()
		percentOmit := math.Trunc(10000*float32(omitted)/float32(e.Evaluations()+omitted)) / 100
		log("evaluated SDF", e.Evaluations(), "times and rendered", len(triangles), "triangles in", watch(), "with", percentOmit, "percent evaluations omitted")

		watch = stopwatch()
		_, err = glrender.WriteBinarySTL(cfg.STLOutput, triangles)
		if err != nil {
			return fmt.Errorf("writing STL file: %s", err)
		}
		filename := "STL"
		if fp, ok := cfg.STLOutput.(*os.File); ok {
			filename = fp.Name()
		}
		log("wrote", filename, "in", watch())
	}

	return nil
}

func stopwatch() func() time.Duration {
	start := time.Now()
	return func() time.Duration {
		return time.Since(start)
	}
}
