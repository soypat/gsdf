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
func scenePotBase() (glbuild.Shader3D, error) {
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
	poly2, err := gsdf.NewPolygon(verts)
	if err != nil {
		return nil, err
	}
	sdf, err := gleval.NewCPUSDF2(poly2)
	if err != nil {
		return nil, err
	}
	err = gsdfaux.RenderPNGFile(visualization2D, sdf, 1080, nil)
	if err != nil {
		return nil, err
	}
	return gsdf.Revolve(poly2, 0)
}

func run() error {
	useGPU := flag.Bool("gpu", false, "Enable GPU usage")
	flag.Parse()
	object, err := scenePotBase()
	if err != nil {
		return err
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
		Resolution:   object.Bounds().Diagonal() / 350,
		UseGPU:       *useGPU,
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
