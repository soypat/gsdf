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
func scene() (glbuild.Shader3D, error) {
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
	// Object accumulates the showerhead sdf.
	var object glbuild.Shader3D

	// startblock := must3.Cylinder(10, threadExtDiameter/2+showerheadWall, 0)
	knurled, err := threads.KnurledHead(threadExtDiameter/2+showerheadWall, threadheight, 1)
	if err != nil {
		return nil, err
	}
	threads, err := threads.Screw(threadheight+.5, showerThread)
	if err != nil {
		return nil, err
	}
	object = gsdf.Difference(knurled, threads)

	base, err := gsdf.NewCylinder(threadExtDiameter/2+showerheadWall, showerheadBaseThick, 0)
	if err != nil {
		return nil, err
	}
	base = gsdf.Translate(base, 0, 0, -(threadedLength/2 + showerheadBaseThick/2 - 1))

	// Make showerhead holesSlice with fibonacci spacing.)
	hole, _ := gsdf.NewCylinder(0.8, showerheadBaseThick*10, 0)
	// Declare Hole accumulator.
	holes := hole
	for i := 0; i < 130; i++ {
		v := fibonacci(i)
		holes = gsdf.Union(holes, gsdf.Translate(hole, v.X, v.Y, 0))
	}
	base = gsdf.Difference(base, holes)

	object = gsdf.Union(object, base)
	return object, nil
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

	err = gsdfaux.Render(object, gsdfaux.RenderConfig{
		STLOutput:     fpstl,
		VisualOutput:  fpvis,
		Resolution:    object.Bounds().Diagonal() / 200,
		UseGPU:        *useGPU,
		EnableCaching: !*useGPU, // Has many unions, part can likely benefit from caching when using CPU.
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
