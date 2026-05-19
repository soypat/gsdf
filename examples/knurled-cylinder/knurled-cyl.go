package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	math32 "github.com/chewxy/math32"
	"github.com/soypat/geometry/ms3"
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
// Matches the following python program step-by-step:
//
//	f = rounded_cylinder(1, 0.1, 5)
//	x = box((1, 1, 4)).rotate(pi / 4)
//	x = x.circular_array(24, 1.6)
//	x = x.twist(0.75) | x.twist(-0.75)
//	f -= x.k(0.1)
//	f -= cylinder(0.5).k(0.1)
//	c = cylinder(0.25).orient(X)
//	f -= c.translate(Z * -2.5).k(0.1)
//	f -= c.translate(Z * 2.5).k(0.1)
func BuildShape(bld *gsdf.Builder, flags Flags) (glbuild.Shader3D, error) {
	r := float32(flags.Diameter / 2)

	length := float32(flags.Length)
	if length == 0 {
		length = 5 * r // Python aspect ratio: length = 5 * radius
	}
	holeDiam := float32(flags.HoleDiam)
	if holeDiam == 0 {
		holeDiam = r // hole radius = r/2, matching Python's cylinder(0.5) at radius 1
	}
	knurlSide := float32(flags.KnurlSize)
	if knurlSide == 0 {
		knurlSide = r // Python box side = 1 at radius 1
	}

	const (
		smoothRatio  = float32(0.1)  // smooth blend radius / cylinder radius (Python .k(0.1))
		twistK       = float32(0.75) // twist radians per unit length at radius 1 (Python twist(0.75))
		knurlOffsetR = float32(1.6)  // radial placement of knurl boxes (Python circular_array offset)
		knurlN       = 24            // knurl instances (Python circular_array n)
	)

	sk := smoothRatio * r // smooth blend in world units

	// Main body: rounded cylinder
	obj := bld.NewCylinder(r, length, smoothRatio*r)

	// Knurling: 45°-rotated box placed at radius 1.6r, repeated 24 times,
	// then unioned with its mirror twist to create diamond knurl pattern.
	knurlBox := bld.NewBox(knurlSide, knurlSide, length*0.8, 0)
	knurlBox = bld.Rotate(knurlBox, math32.Pi/4, ms3.Vec{Z: 1})
	knurlBox = bld.Translate(knurlBox, knurlOffsetR*r, 0, 0)
	knurlBox = bld.CircularArray(knurlBox, knurlN, knurlN)
	knurl := bld.Union(
		bld.Twist(knurlBox, twistK/r),
		bld.Twist(knurlBox, -twistK/r),
	)
	obj = bld.SmoothDifference(sk, obj, knurl)

	// Central through-hole
	obj = bld.SmoothDifference(sk, obj, bld.NewCylinder(holeDiam/2, length+2*r, 0))

	// Vent holes along X axis at each end face
	ventCyl := bld.NewCylinder(0.25*r, 3*r, 0)
	ventCyl = bld.Rotate(ventCyl, math32.Pi/2, ms3.Vec{Y: 1})
	obj = bld.SmoothDifference(sk, obj, bld.Translate(ventCyl, 0, 0, -length/2))
	obj = bld.SmoothDifference(sk, obj, bld.Translate(ventCyl, 0, 0, length/2))

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
	err = gsdfaux.UI(sdf, gsdfaux.UIConfig{
		Width:  1024,
		Height: 800,
	})
	return nil
}

func main() {
	log.Println("start")
	if err := run(); err != nil {
		log.Fatal(err)
	}
	log.Println("success")
}
