// DO NOT EDIT
// this is a template to base your
// one-file scripts off of.
// Copy the contents of this file to a new directory and run.

package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

// Required if using GPU to render shapes or using UI.
func init() { runtime.LockOSThread() }

const defaultName = "mop-joint" // See [Flags.Name]

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

// Measurements in [mm]
const (
	// All measurements done from arced bottom, but we center on shaft coordinates.
	shaftDiameter       = 5.8
	shaftLen            = 34.0
	shaftCenter         = shaftDiameter/2 + 1
	shaftTrimProtrusion = 5.24

	baseHeight = 23.0 - shaftCenter

	shaftToBaseTop = baseHeight - shaftCenter
	holeX          = 10.0
	holeY          = 13.85 - shaftCenter
	holeDiameter   = 4.1

	baseWidth          = 20.5
	baseWidthSideTrim  = 4.3
	baseStraightHeight = 15.3 - shaftCenter

	baseSideTrimRadius = 4.3
	baseTrimHeight     = 13.51
	baseThick          = 10.7
)

// BuildShape receives the parameters to define the shape requested by user.
func BuildShape(bld *gsdf.Builder, flags Flags) (obj glbuild.Shader3D, err error) {
	draw := buildBaseFrontDrawing(bld)
	sdf2, err := gleval.NewCPUSDF2(draw)
	if err != nil {
		return nil, err
	}
	err = gsdfaux.RenderPNGFile("mop-top.png", sdf2, 500, gsdfaux.ColorConversionInigoQuilez(sdf2.Bounds().Diagonal()))
	if err != nil {
		return nil, err
	}
	obj = bld.Extrude(draw, baseThick)

	// We need to round lower base along X with an arc in YZ-plane, arc radius baseThick.
	shaft := bld.NewCylinder(shaftDiameter/2, shaftLen, shaftDiameter/8)
	shaftEncaps := bld.NewCylinder(baseThick/2, baseWidth, 3)
	shaft = bld.SmoothUnion(2, shaft, shaftEncaps)

	shaft = bld.Rotate(shaft, math.Pi/2, ms3.Vec{Y: 1})
	shaft = bld.Translate(shaft, shaftLen/2-shaftTrimProtrusion-2, 0, 0)

	obj = bld.Union(obj, shaft)
	return obj, nil
}

func buildBaseFrontDrawing(bld *gsdf.Builder) (obj glbuild.Shader2D) {
	var poly ms2.PolygonBuilder
	poly.Add(ms2.Vec{}) // Origin lower left.
	poly.AddRelativeXY(baseWidth, 0)
	src := poly.AddRelativeXY(0, baseStraightHeight).Position()
	// now interpolate with bezier cubic between pos and
	tgt := ms2.Vec{X: baseWidth / 2.0, Y: baseHeight}
	sampler := ms2.Spline3Sampler{
		Spline:    ms2.SplineBezierCubic(),
		Tolerance: 1e-7,
	}
	const splineSoften = 4
	sampler.SetSplinePoints(src, ms2.Vec{X: src.X, Y: tgt.Y - splineSoften}, ms2.Vec{X: src.X - splineSoften, Y: tgt.Y}, tgt)
	bezier := sampler.SampleBisect(nil, 3)
	for _, v := range bezier {
		poly.Add(v)
	}
	fmt.Println("bezier length:", len(bezier))
	poly.Add(tgt)
	poly.AddRelativeXY(-baseSideTrimRadius-1, 0).Smooth(baseSideTrimRadius, 7)
	poly.AddRelativeXY(0, -baseTrimHeight)
	poly.Add(ms2.Vec{Y: poly.Last().Position().Y})
	v, err := poly.AppendVecs(nil)
	if err != nil {
		panic(err)
	}
	shape := bld.NewPolygon(v)
	shape = bld.Difference2D(shape, bld.Translate2D(bld.NewCircle(holeDiameter/2), holeX, holeY))
	return shape
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
	if flags.UseGPU {

	}
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
