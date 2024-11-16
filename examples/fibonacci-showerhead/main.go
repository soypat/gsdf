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
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

const (
	stl           = "showerhead.stl"
	visualization = "showerhead.glsl"
)

func init() {
	runtime.LockOSThread() // In case we wish to use OpenGL.
}

// scene returns the showerhead object.
func scene(bld *gsdf.Builder) (glbuild.Shader3D, error) {
	// Showerhead parameters as defined by showerhead geometry.
	const (
		threadExtDiameter = 65.
		threadedLength    = 5.
		threadTurns       = 3.
		threadPitch       = threadedLength / threadTurns
	)

	// Constructuive parameters defined by our design.
	const (
		showerheadBaseThick = 2.5
		showerheadWall      = 4.
		threadheight        = 5.
	)

	var (
		showerThread = threads.PlasticButtress{
			D: threadExtDiameter,
			P: threadPitch,
		}
	)
	t2d, err := showerThread.Thread(bld)
	if err != nil {
		return nil, err
	}
	threadSDF, _ := gleval.NewCPUSDF2(t2d)
	gsdfaux.RenderPNGFile("thread.png", threadSDF, 512, nil)

	// Object accumulates the showerhead sdf.
	var object glbuild.Shader3D

	// startblock := must3.Cylinder(10, threadExtDiameter/2+showerheadWall, 0)
	knurled, err := threads.KnurledHead(bld, threadExtDiameter/2+showerheadWall, threadheight, 1)
	if err != nil {
		return nil, err
	}

	threads, err := threads.Screw(bld, threadheight+.5, showerThread)
	if err != nil {
		return nil, err
	}
	object = bld.Difference(knurled, threads)
	base := bld.NewCylinder(threadExtDiameter/2+showerheadWall, showerheadBaseThick, 0)
	base = bld.Translate(base, 0, 0, -(threadedLength/2 + showerheadBaseThick/2 - 1))

	// Make showerhead holesSlice with fibonacci spacing.)
	hole := bld.NewCylinder(0.8, showerheadBaseThick*10, 0)
	// Declare Hole accumulator.
	holes := hole
	for i := 0; i < 130; i++ {
		v := fibonacci(i)
		holes = bld.Union(holes, bld.Translate(hole, v.X, v.Y, 0))
	}
	base = bld.Difference(base, holes)

	object = bld.Union(object, base)
	return object, bld.Err()
}

func run() error {
	var (
		useGPU     bool
		resolution float64
		flagResDiv uint
	)
	flag.BoolVar(&useGPU, "gpu", false, "enable GPU usage")
	flag.Float64Var(&resolution, "res", 0, "Set resolution in shape units. Useful for setting the minimum level of detail to a fixed amount for final result. If not set resdiv used [mm/in]")
	flag.UintVar(&flagResDiv, "resdiv", 200, "Set resolution in bounding box diagonal divisions. Useful for prototyping when constant speed of rendering is desired.")
	flag.Parse()
	var bld gsdf.Builder
	object, err := scene(&bld)
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
		STLOutput:     fpstl,
		VisualOutput:  fpvis,
		Resolution:    float32(resolution),
		UseGPU:        useGPU,
		EnableCaching: !useGPU, // Has many unions, part can likely benefit from caching when using CPU.
	})

	return err
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("showerhead example done")
}

func fibonacci(n int) ms2.Vec {
	// Angle of divergence is very sensitive- 137.3 to 137.5 varies pattern greatly.
	const angleOfDivergence = 137.3
	const spacing = 2.6
	nf := float32(n)
	a := nf * angleOfDivergence / 360 * math.Pi
	r := spacing * math.Sqrt(nf)
	sa, ca := math.Sincos(a)
	return ms2.Vec{X: r * ca, Y: r * sa}
}
