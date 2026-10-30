package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

// Required if using GPU to render shapes or using UI.
func init() { runtime.LockOSThread() }

const defaultName = "icicle-kit-support" // See [Flags.Name]

type Flags struct {
	// Below are rendering configuration flags.

	Name                string  // name used for file output names and logging.
	UseGPU              bool    // enables GPU usage for rendering STL file if requested.
	Resolution          float64 // If non-zero will define the minimum level of detail of shape. Lower=finer resolution. Overrides ResolutionDivisions.
	ResolutionDivisions uint    // If Resolution not set this will define number of subdivisions of SDF dominion. Higher=finer resolution.

	// Below are shape definitions.
	// This is what you change to fit your needs!

}

func (f Flags) SDFResolution(diagonal float64) float64 {
	if f.Resolution == 0 {
		return diagonal / float64(f.ResolutionDivisions)
	}
	return f.Resolution
}

// Devkit constructive parameters.
const (
	drillDiam          = 2.75
	boardRadius        = 2.5
	maxCapacitorHeight = 12 // highest capacitor on devkit underside.
)

// Support constructive parameters millimeters.
const (
	supportThick     = 4.0
	thruHoleDiam     = drillDiam
	pillarDiam       = thruHoleDiam * 2.75
	pillarHeight     = maxCapacitorHeight + 4
	pillarRootSmooth = 2
)

var (
	// Position of hole centers in millimeters.
	holeCoords = [][2]float32{
		{0, 0}, {165.2, 0}, {169.5, 115.2}, {0, 117.3}, // four extreme corners.
		{31.1, 0}, {111.5, 115.2}, // lower and top midpoint holes.
		{80.2, 23.9}, {147.8, 26.2}, // lower central holes.
		{14.2, 62.7}, {111.1, 68}, // upper central holes.
	}
	boardBox = ms2.Box{
		Min: ms2.Vec{X: -4.2, Y: -4.2},
		Max: ms2.Vec{X: 178.7, Y: 121.5},
	}
)

func buildSupportOutline(bld *gsdf.Builder) (obj glbuild.Shader2D, err error) {
	dims := boardBox.Size()
	outline := bld.NewRectangle(dims.X, dims.Y, boardRadius)
	// box created and centered at (0,0), need to move it to match coords.
	center := boardBox.Center()
	outline = bld.Translate2D(outline, center.X, center.Y)
	hole := bld.NewCircle(drillDiam / 2)
	var holes []glbuild.Shader2D
	for _, coords := range holeCoords {
		holes = append(holes, bld.Translate2D(hole, coords[0], coords[1]))
	}
	holesUnion := bld.Union2D(holes...)
	outline = bld.Difference2D(outline, holesUnion)
	return outline, nil
}

// BuildShape receives the parameters to define the shape requested by user.
func BuildShape(bld *gsdf.Builder, flags Flags) (obj glbuild.Shader3D, err error) {
	// Start with 2D model of base with holes.
	baseOutline, err := buildSupportOutline(bld)
	if err != nil {
		return nil, err
	}
	sdf2, err := gleval.NewCPUSDF2(baseOutline)
	if err != nil {
		return nil, err
	}
	err = gsdfaux.RenderPNGFile(flags.Name+".png", sdf2, 500, gsdfaux.ColorConversionInigoQuilez(sdf2.Bounds().Diagonal()))
	if err != nil {
		return nil, err
	}
	// Begin 3D model.
	obj = bld.Extrude(baseOutline, supportThick)
	// Model the pillars. The pillar hole will be substracted at the end so smooth unions don't occupy internal space.
	pillar := bld.NewCylinder(pillarDiam/2, pillarHeight, 0)
	const pillarz = supportThick/2 + pillarHeight/2
	var pillars []glbuild.Shader3D
	for _, coords := range holeCoords {
		pillars = append(pillars, bld.Translate(pillar, coords[0], coords[1], pillarz))
	}
	pillarUnion := bld.Union(pillars...)
	obj = bld.SmoothUnion(pillarRootSmooth, obj, pillarUnion)

	thruHole := bld.NewCylinder(thruHoleDiam/2, 100, 0)
	var thruHoles []glbuild.Shader3D
	for _, coords := range holeCoords {
		thruHoles = append(thruHoles, bld.Translate(thruHole, coords[0], coords[1], pillarz))
	}
	holeUnion := bld.Union(thruHoles...)
	obj = bld.Difference(obj, holeUnion)
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
	// flag.Float64Var(&flags.Diameter, "d", 20, "Diameter of cylinder.")
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
