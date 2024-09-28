package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"time"

	"os"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

const (
	stl             = "gasket.stl"
	visualization   = "gasket.glsl"
	visualization2D = "gasket2D.png"
)

func init() {
	runtime.LockOSThread() // In case we wish to use OpenGL.
}

// scene returns the gasket object.
func scene() (glbuild.Shader3D, error) {
	// Sistema Food Storage Container geometry definitions.
	// The problem we are trying to solve is how the container is not airtight
	// due to the o-ring not sealing against the lid. We can aid the o-ring
	// by adding a gasket that sits in the lid so that it fills the empty space
	// between lid and o-ring. This particular example is for the common 1 liter tupper.
	const (
		tupperW             = 96.
		tupperL             = 156.
		tupperLStartRound   = 154.
		channelW            = 4.15
		round               = 10.
		extRound            = round + 1.2*channelW
		channelWall         = 0.78
		tupperLArcRadius    = tupperL * 2.4
		extTupperLArcRadius = tupperLArcRadius + channelW
	)
	// Our gasket constructive geometry definitions.
	const (
		gasketHeight = 1
		tol          = 0.8     // remove material from channel walls.
		eps          = 1 + tol // Prevent from offset from opening symmetry edges.
	)

	var poly ms2.PolygonBuilder
	poly.AddXY(tupperL/2, -eps)
	poly.AddXY(tupperLStartRound/2, tupperW/2-round).Arc(tupperLArcRadius, 5)
	poly.AddXY(tupperLStartRound/2-round, tupperW/2).Arc(round, 6)
	poly.AddXY(-eps, tupperW/2)
	poly.AddXY(-eps, tupperW/2+channelW)
	poly.AddXY(tupperLStartRound/2-round, tupperW/2+channelW)
	poly.AddXY(tupperLStartRound/2+channelW, tupperW/2-2*channelW).Arc(-extRound, 6)
	poly.AddXY(tupperL/2+channelW, -eps).Arc(-extTupperLArcRadius, 5)

	verts, err := poly.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	poly2, err := gsdf.NewPolygon(verts)
	if err != nil {
		return nil, err
	}
	poly2 = gsdf.Symmetry2D(poly2, true, true)
	poly2 = gsdf.Offset2D(poly2, tol)
	if visualization2D != "" {
		start := time.Now()
		sdf, err := gleval.NewCPUSDF2(poly2)
		if err != nil {
			return nil, err
		}
		err = gsdfaux.RenderPNGFile(visualization2D, sdf, 500, nil)
		if err != nil {
			return nil, err
		}
		fmt.Println("wrote 2D visualization to", visualization2D, "in", time.Since(start))
	}
	return gsdf.Extrude(poly2, gasketHeight)
}

func run() error {
	useGPU := flag.Bool("gpu", false, "Enable GPU usage")
	flag.Parse()
	object, err := scene()
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
	fmt.Println("gasket example done")
}
