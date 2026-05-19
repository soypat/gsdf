package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

func init() { runtime.LockOSThread() } // Required if using GPU to render shapes or using UI.

const defaultName = "knurled-cyl" // See [Flags.Name]

type Flags struct {
	// Below are rendering configuration flags.

	Name                string  // name used for file output names and logging.
	UseGPU              bool    // enables GPU usage for rendering STL file if requested.
	Resolution          float64 // If non-zero will define the minimum level of detail of shape. Lower=finer resolution. Overrides ResolutionDivisions.
	ResolutionDivisions uint    // If Resolution not set this will define number of subdivisions of SDF dominion. Higher=finer resolution.

	// Below are shape definitions.
	// This is what you change to fit your needs!

	Diameter  float64
	HoleDiam  float64
	Length    float64
	KnurlSize float64
}

func (f Flags) SDFResolution(diagonal float64) float64 {
	if f.Resolution == 0 {
		return diagonal / float64(f.ResolutionDivisions)
	}
	return f.Resolution
}

// BuildShape receives the parameters to define the shape requested by user.
func BuildShape(bld *gsdf.Builder, flags Flags) (glbuild.Shader3D, error) {
	/*
		# Equivalent python program:
		# main body
		f = rounded_cylinder(1, 0.1, 5)

		# knurling
		x = box((1, 1, 4)).rotate(pi / 4)
		x = x.circular_array(24, 1.6)
		x = x.twist(0.75) | x.twist(-0.75)
		f -= x.k(0.1)

		# central hole
		f -= cylinder(0.5).k(0.1)

		# vent holes
		c = cylinder(0.25).orient(X)
		f -= c.translate(Z * -2.5).k(0.1)
		f -= c.translate(Z * 2.5).k(0.1)

		f.save('knurling.stl', samples=2**26)
	*/
	var obj glbuild.Shader3D

	return obj, bld.Err()
}

func run() error {
	var flags Flags

	// Rendering config:
	flag.StringVar(&flags.Name, "name", defaultName, "Name of shape. Used for filenames and logging.")
	flag.BoolVar(&flags.UseGPU, "gpu", false, "enable GPU usage")
	flag.Float64Var(&flags.Resolution, "res", 0, "Set resolution in shape units. Useful for setting the minimum level of detail to a fixed amount for final result. If not set resdiv used [mm/in]")
	flag.UintVar(&flags.ResolutionDivisions, "resdiv", 200, "Set resolution in bounding box diagonal divisions. Useful for prototyping when constant speed of rendering is desired.")

	// Shape config:
	flag.Float64Var(&flags.Diameter, "d", 20, "Diameter of cylinder.")
	flag.Parse()

	var bld gsdf.Builder
	bld.SetFlags(gsdf.FlagNoShaderBuffers)
	sdf, err := BuildShape(&bld, flags)
	if err != nil {
		return fmt.Errorf("while defining %q shape: %w", flags.Name, err)
	}
	resolution := flags.SDFResolution(float64(sdf.Bounds().Diagonal()))
	fpstl, err := os.Create(flags.Name + ".stl")
	if err != nil {
		return err
	}
	defer fpstl.Close()

	err = gsdfaux.RenderShader3D(sdf, gsdfaux.RenderConfig{
		STLOutput:  fpstl,
		Resolution: float32(resolution),
		UseGPU:     flags.UseGPU,
	})
	if err != nil {
		return fmt.Errorf("while rendering %q shape: %w", flags.Name, err)
	}
	return nil
}

func main() {
	log.Println("start")
	if err := run(); err != nil {
		log.Fatal(err)
	}
	log.Println("success")
}
