package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

func init() {
	runtime.LockOSThread() // For when using GPU this is required.
}

func buildSpacer(bld *gsdf.Builder, holeDiameter, length float32) (glbuild.Shader3D, error) {
	// hexFaceToFace := math.Ceil(holeDiameter*1.4) - 0.15
	// hexRadius := hexFaceToFace / math.Cos(30.*math.Pi/180.) / 2
	hex := bld.NewHexagon(holeDiameter * 1.15)
	sdf, err := gleval.NewCPUSDF2(hex)
	hex = bld.Difference2D(hex, bld.NewCircle(holeDiameter/2))
	hex3d := bld.Extrude(hex, length)

	if err != nil {
		return nil, err
	}
	gsdfaux.RenderPNGFile(fmt.Sprintf("M%gx%g.png", holeDiameter, length), sdf, 1000, nil)
	return hex3d, bld.Err()
}

func run() error {
	var (
		useGPU        bool
		resolution    float64
		flagResDiv    uint
		flagSpacers   string
		scaleDiameter float64
		writeIRMF     bool
	)
	flag.BoolVar(&useGPU, "gpu", false, "enable GPU usage")
	flag.BoolVar(&writeIRMF, "irmf", false, "write IRMF file")
	flag.Float64Var(&resolution, "res", 0, "Set resolution in shape units. Useful for setting the minimum level of detail to a fixed amount for final result. If not set resdiv used [mm/in]")
	flag.UintVar(&flagResDiv, "resdiv", 200, "Set resolution in bounding box diagonal divisions. Useful for prototyping when constant speed of rendering is desired.")
	flag.StringVar(&flagSpacers, "spacers", "M3x5", "Spacers to generate with format arg[,arg] where arg has format M<d>x<L> d is diameter of hole and L is length.")
	flag.Float64Var(&scaleDiameter, "dscale", 1, "Scale diameter of spacers.")
	flag.Parse()
	strSpacers := strings.Split(flagSpacers, ",")
	if len(strSpacers) == 0 {
		flag.PrintDefaults()
		return errors.New("invalid spacers")
	}
	if scaleDiameter <= 0 {
		return errors.New("invalid diameter scale parameter")
	}
	log.Println("start program")
	var bld gsdf.Builder
	bld.SetFlags(gsdf.FlagNoShaderBuffers)
	var sdfs []glbuild.Shader3D
	for _, strSpacer := range strSpacers {
		if len(strSpacer) == 0 {
			return errors.New("empty spacer argument")
		} else if strSpacer[0] != 'M' {
			return errors.New("spacer arg must start with M")
		}
		strDiam, strLength, ok := strings.Cut(strSpacer[1:], "x")
		if !ok {
			return errors.New("did not find 'x' separator in spacer arg")
		}
		d, err := strconv.ParseFloat(strDiam, 32)
		if err != nil {
			return err
		}
		L, err := strconv.ParseFloat(strLength, 32)
		if err != nil {
			return err
		}
		diamCorrected := d * scaleDiameter
		sdf, err := buildSpacer(&bld, float32(diamCorrected), float32(L))
		if err != nil {
			return fmt.Errorf("building spacer %s: %w", strSpacer, err)
		}
		sdfs = append(sdfs, sdf)
	}

	for i, sdf := range sdfs {
		fpstl, err := os.Create(strSpacers[i] + ".stl")
		if err != nil {
			return err
		}
		var fpirmf *os.File
		if writeIRMF {
			fpirmf, err = os.Create(strSpacers[i] + ".irmf")
			if err != nil {
				fpstl.Close()
				return err
			}
		}

		if resolution == 0 {
			resolution = float64(sdf.Bounds().Diagonal()) / float64(flagResDiv)
		}
		err = gsdfaux.RenderShader3D(sdf, gsdfaux.RenderConfig{
			STLOutput:  fpstl,
			IRMFOutput: fpirmf,
			Resolution: float32(resolution),
			UseGPU:     useGPU,
		})
		fpstl.Close()
		if fpirmf != nil {
			fpirmf.Close()
		}
	}
	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("bolt example done")
}
