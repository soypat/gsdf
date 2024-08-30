package gsdfaux

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"

	"time"

	"github.com/chewxy/math32"
	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms1"
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
		envelope, err := gsdf.NewBoundsBoxFrame(bb)
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

// RenderPNGFile renders a 2D SDF as an image and saves result to a PNG file with said filename.
// The image width is sized automatically from the image height argument to preserve SDF aspect ratio.
// If a nil color conversion function is passed then one is automatically chosen.
func RenderPNGFile(filename string, s glbuild.Shader2D, picHeight int, useGPU bool, colorConversion func(float32) color.Color) error {
	if useGPU {
		return errors.New("TODO: implement GPU rendering for RenderPNGFile")
	}
	bb := s.Bounds()
	sz := bb.Size()
	if colorConversion == nil {
		colorConversion = ColorConversionInigoQuilez(bb.Diagonal() / 3)
	}
	pixPerUnit := float64(picHeight) / float64(sz.Y)
	picWidth := int(pixPerUnit * float64(sz.X))
	img := image.NewRGBA(image.Rect(0, 0, picWidth, picHeight))
	renderer, err := glrender.NewImageRendererSDF2(max(4096, picWidth), colorConversion)
	if err != nil {
		return err
	}

	sdf, err := gleval.NewCPUSDF2(s)
	if err != nil {
		return err
	}
	err = renderer.Render(sdf, img, nil)
	if err != nil {
		return err
	}
	fp, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer fp.Close()
	err = png.Encode(fp, img)
	if err != nil {
		return err
	}
	fp.Sync()
	return nil
}

// ColorConversionInigoQuilez creates a new color conversion using [Inigo Quilez]'s style.
// A good value for characteristic distance is the bounding box diagonal divided by 3.
//
// [Inigo Quilez]: https://iquilezles.org/articles/distfunctions2d/
func ColorConversionInigoQuilez(characteristicDistance float32) func(float32) color.Color {
	inv := 1. / characteristicDistance
	return func(d float32) color.Color {
		d *= inv
		var one = ms3.Vec{X: 1, Y: 1, Z: 1}
		var c ms3.Vec
		if d > 0 {
			c = ms3.Vec{X: 0.9, Y: 0.6, Z: 0.3}
		} else {
			c = ms3.Vec{X: 0.65, Y: 0.85, Z: 1.0}
		}
		c = ms3.Scale(1-math32.Exp(-6*math32.Abs(d)), c)
		c = ms3.Scale(0.8+0.2*math32.Cos(150*d), c)
		max := 1 - ms1.SmoothStep(0, 0.01, math32.Abs(d))
		c = ms3.InterpElem(c, one, ms3.Vec{X: max, Y: max, Z: max})
		return color.RGBA{
			R: uint8(c.X * 255),
			G: uint8(c.Y * 255),
			B: uint8(c.Z * 255),
			A: 255,
		}
	}
}
