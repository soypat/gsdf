// DO NOT EDIT
// this is a template to base your
// one-file scripts off of.
// Copy the contents of this file to a new directory and run.

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

// Required if using GPU to render shapes or using UI.
func init() { runtime.LockOSThread() }

const defaultName = "template" // See [Flags.Name]

type Flags struct {
	// Below are rendering configuration flags.

	Name                string  // name used for file output names and logging.
	UseGPU              bool    // enables GPU usage for rendering STL file if requested.
	Resolution          float64 // If non-zero will define the minimum level of detail of shape. Lower=finer resolution. Overrides ResolutionDivisions.
	ResolutionDivisions uint    // If Resolution not set this will define number of subdivisions of SDF dominion. Higher=finer resolution.

	// Below are shape definitions.
	// This is what you change to fit your needs!

	Diameter float64
}

func (f Flags) SDFResolution(diagonal float64) float64 {
	if f.Resolution == 0 {
		return diagonal / float64(f.ResolutionDivisions)
	}
	return f.Resolution
}

// BuildShape receives the parameters to define the shape requested by user.
func BuildShape(bld *gsdf.Builder, flags Flags) (glbuild.Shader3D, error) {
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
