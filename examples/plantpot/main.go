package main

import (
	"flag"
	"fmt"
	"log"

	"runtime"

	"os"

	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

const (
	exampleName     = "plantpot"
	stl             = exampleName + ".stl"
	visualization   = exampleName + ".glsl"
	visualization2D = exampleName + ".png"
	potBaseRadius   = 40.
)

func init() {
	runtime.LockOSThread() // In case we wish to use OpenGL.
}

// scenePotBase returns the plant pot base object.
func scenePotBase(bld *gsdf.Builder) (glbuild.Shader3D, error) {
	const (
		baseHeight         = 10.
		baseInclinationDeg = 45.
		baseInclination    = baseInclinationDeg * math.Pi / 180 //Convert to radians.
		baseWallThick      = 5.
		baseLipRadius      = baseWallThick * .54
	)

	xOff := baseHeight * math.Sin(baseInclination)
	var poly ms2.PolygonBuilder
	poly.AddXY(0, 0)
	poly.AddXY(potBaseRadius, 0)
	poly.AddXY(potBaseRadius+xOff, baseHeight)
	poly.AddRelativeXY(baseWallThick/3, -baseWallThick).Arc(-baseLipRadius, 20)
	poly.AddXY(potBaseRadius+baseWallThick/2, -baseWallThick)
	poly.AddXY(0, -baseWallThick)
	verts, err := poly.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	poly2 := bld.NewPolygon(verts)

	sdf, err := gleval.NewCPUSDF2(poly2)
	if err != nil {
		return nil, err
	}
	err = gsdfaux.RenderPNGFile(visualization2D, sdf, 1080, nil)
	if err != nil {
		return nil, err
	}
	return bld.Revolve(poly2, 0), bld.Err()
}

func run() error {
	var (
		useGPU     bool
		resolution float64
		flagResDiv uint
	)
	flag.BoolVar(&useGPU, "gpu", false, "enable GPU usage")
	flag.Float64Var(&resolution, "res", 0, "Set resolution in shape units. Useful for setting the minimum level of detail to a fixed amount for final result. If not set resdiv used [mm/in]")
	flag.UintVar(&flagResDiv, "resdiv", 350, "Set resolution in bounding box diagonal divisions. Useful for prototyping when constant speed of rendering is desired.")
	flag.Parse()
	var bld gsdf.Builder
	object, err := scenePotBase(&bld)
	if err != nil {
		return err
	}
	if resolution == 0 {
		resolution = float64(object.Bounds().Diagonal()) / float64(flagResDiv)
	}
	fpstl, err := os.Create(stl)
	if err != nil {
		return err
	}
	defer fpstl.Close()
	fpvis, err := os.Create(visualization)
	if err != nil {
		return err
	}
	defer fpvis.Close()

	err = gsdfaux.RenderShader3D(object, gsdfaux.RenderConfig{
		STLOutput:    fpstl,
		VisualOutput: fpvis,
		Resolution:   float32(resolution),
		UseGPU:       useGPU,
	})

	return err
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(exampleName, "example done")
}
