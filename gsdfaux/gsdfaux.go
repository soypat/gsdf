package gsdfaux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"slices"

	"time"

	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/glrender"
)

type RenderConfig struct {
	STLOutput    io.Writer
	VisualOutput io.Writer
	// Resolution decides the STL output resolution. It correlates with the minimum triangle size.
	Resolution float32
	UseGPU     bool
	Silent     bool
	// EnableCaching uses [gleval.BlockCachedSDF3] to omit potential evaluations.
	// Can cut down on times for very complex SDFs, mainly when using CPU.
	EnableCaching bool
	builder       *gsdf.Builder
}

type UIConfig struct {
	Width, Height int
	Context       context.Context
}

func UI(s glbuild.Shader3D, cfg UIConfig) error {
	if s == nil {
		return errors.New("nil shader")
	}
	return ui(s, cfg)
}

// RenderShader3D is an auxiliary function to aid users in getting setup in using gsdf quickly.
// Ideally users should implement their own rendering functions since applications may vary widely.
func RenderShader3D(s glbuild.Shader3D, cfg RenderConfig) (err error) {
	if cfg.Resolution <= 0 && !math.IsNaN(cfg.Resolution) && !math.IsInf(cfg.Resolution, 0) {
		return errors.New("RenderConfig resolution must be positive, non-infinity")
	}
	if cfg.builder == nil {
		cfg.builder = &gsdf.Builder{}
	}
	bld := cfg.builder
	if cfg.STLOutput == nil && cfg.VisualOutput == nil {
		return errors.New("RenderShader3D requires output parameter in config")
	}
	log := func(args ...any) {
		if !cfg.Silent {
			fmt.Println(args...)
		}
	}
	logDuration := func(duration time.Duration, args ...any) {
		switch {
		case duration > time.Minute:
			duration = duration.Round(time.Second)
		case duration > time.Second:
			duration = duration.Round(time.Millisecond)
		case duration > time.Millisecond:
			duration = duration.Round(time.Microsecond)
		}
		args = append([]any{fmt.Sprintf("%9s", duration.String())}, args...)
		log(args...)
	}
	err = glbuild.ShortenNames3D(&s, 12)
	if err != nil {
		return fmt.Errorf("shortening shader names: %s", err)
	}

	bufferEvalSize := 4096 // Default "sensible" value.
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
		// We set the size of our buffer based on the limitations of
		// GPU hardware. GPUs have compute work groups which run invocations(warps/threads).
		// Workers run in parallel, which in turn run the invocations within their work group in parallel.
		// Typically the number of parallel work groups run in parallel runs between 20 and 64 on modern GPUs.
		// OpenGL does not expose an API to calculate this number so we take a best guess.
		// 32 was the optimal guess for a AMD ATI Radeon RX 6800, with 1024 invocations, resulting in 32678 size of buffer (32*32*32).
		const guessedNumberOfParallelWorkers = 32
		invoc := glgl.MaxComputeInvocations()
		bufferEvalSize = invoc * guessedNumberOfParallelWorkers

		source := new(bytes.Buffer)
		prog := glbuild.NewDefaultProgrammer()

		log("compute invocation size ", invoc)
		if invoc < 1 {
			return errors.New("zero or negative GPU invocation size")
		}
		prog.SetComputeInvocations(invoc, 1, 1)
		var objects []glbuild.ShaderObject
		_, objects, err = prog.WriteComputeSDF3(source, s)
		if err != nil {
			return err
		}
		sdf, err = gleval.NewComputeGPUSDF3(source, bb, gleval.ComputeConfig{
			InvocX:        invoc,
			ShaderObjects: objects,
		})
	} else {
		log("using CPU")
		cpusdf, err := gleval.NewCPUSDF3(s)
		if err != nil {
			return err
		}
		// Ensure a regular buffer size so Renderer does not inadvertently allocate lots of small buffers.
		cpusdf.VecPool().SetMinAllocationLen(bufferEvalSize)
		sdf = cpusdf
	}

	if err != nil || sdf == nil {
		return fmt.Errorf("instantiating SDF: %s", err)
	}
	if cfg.EnableCaching {
		var cache gleval.BlockCachedSDF3
		cacheRes := cfg.Resolution / 2
		err = cache.Reset(sdf, cacheRes, cacheRes, cacheRes)
		if err != nil {
			return err
		}
		sdf = &cache
		defer func() {
			pcnt := percentUint64(cache.CacheHits(), cache.Evaluations())
			log("SDF caching omitted", pcnt, "percent of", cache.Evaluations(), "SDF evaluations")
		}()
	}
	logDuration(watch(), "instantiating evaluation SDF")
	renderer, err := glrender.NewOctreeRenderer(sdf, cfg.Resolution, bufferEvalSize)
	if err != nil {
		return err
	}

	if cfg.VisualOutput != nil {
		watch = stopwatch()
		const sceneSize = 1.4
		// We include the bounding box in the visualization.
		bb := s.Bounds()
		envelope := bld.NewBoundsBoxFrame(bb)

		visual := bld.Union(s, envelope)
		// Scale size and translate to center so visualization is in camera range.
		center := bb.Center()
		sz := bb.Size()
		visual = bld.Translate(visual, center.X, center.Y, center.Z-sz.Z)
		visual = bld.Scale(visual, sceneSize/bb.Diagonal())
		var objects []glbuild.ShaderObject
		_, objects, err = glbuild.NewDefaultProgrammer().WriteShaderToyVisualizerSDF3(cfg.VisualOutput, visual)
		if err != nil {
			return fmt.Errorf("writing visual GLSL: %s", err)
		} else if slices.ContainsFunc(objects, func(b glbuild.ShaderObject) bool { return b.IsBindable() }) {
			return errors.New("bindable shader objects unsupported for visual outputs")
		}

		filename := "GLSL visualization"
		if fp, ok := cfg.VisualOutput.(*os.File); ok {
			filename = fp.Name()
		}
		logDuration(watch(), "wrote", filename)
	}

	if cfg.STLOutput != nil {
		var userData any
		maybeVP, err := gleval.GetVecPool(sdf)
		if err == nil {
			userData = maybeVP
		}

		watch = stopwatch()
		triangles, err := glrender.RenderAll(renderer, userData)
		if err != nil {
			return fmt.Errorf("rendering triangles: %s", err)
		}

		e := sdf.(interface{ Evaluations() uint64 })
		omitted := 8 * renderer.TotalPruned()
		percentOmit := percentUint64(omitted, e.Evaluations()+omitted)
		logDuration(watch(), "evaluated SDF", e.Evaluations(), "times and rendered", len(triangles), "triangles with", percentOmit, "percent evaluations omitted")

		watch = stopwatch()
		_, err = glrender.WriteBinarySTL(cfg.STLOutput, triangles)
		if err != nil {
			return fmt.Errorf("writing STL file: %s", err)
		}
		filename := "STL"
		if fp, ok := cfg.STLOutput.(*os.File); ok {
			filename = fp.Name()
		}
		logDuration(watch(), "wrote", filename)
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
func RenderPNGFile(filename string, sdf gleval.SDF2, picHeight int, colorConversion func(float32) color.Color) error {
	bb := sdf.Bounds()
	if colorConversion == nil {
		colorConversion = ColorConversionInigoQuilez(bb.Diagonal() / 3)
	}
	sz := bb.Size()
	pixPerUnit := float64(picHeight) / float64(sz.Y)
	picWidth := int(pixPerUnit * float64(sz.X))
	img := image.NewRGBA(image.Rect(0, 0, picWidth, picHeight))
	renderer, err := glrender.NewImageRendererSDF2(max(4096, picWidth), colorConversion)
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

func MakeGPUSDF2(s glbuild.Shader2D) (sdf gleval.SDF2, err error) {
	var n int
	var buf bytes.Buffer
	err = glbuild.ShortenNames2D(&s, 8)
	if err != nil {
		return nil, err
	}
	invoc := glgl.MaxComputeInvocations()
	if invoc < 1 {
		return nil, errors.New("zero or negative GPU invocation size")
	}
	prog := glbuild.NewDefaultProgrammer()
	prog.SetComputeInvocations(invoc, 1, 1)

	var objects []glbuild.ShaderObject
	n, objects, err = prog.WriteComputeSDF2(&buf, s)
	if err != nil {
		return nil, err
	} else if n != buf.Len() {
		return nil, fmt.Errorf("wrote %d bytes but WriteComputeSDF2 counted %d", buf.Len(), n)
	}

	return gleval.NewComputeGPUSDF2(&buf, s.Bounds(), gleval.ComputeConfig{
		InvocX:        invoc,
		ShaderObjects: objects,
	})
}
