package main

import (
	"bytes"
	"fmt"
	"log"
	"runtime"
	"time"

	"os"

	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/glrender"
)

// Showerhead parameters as defined by showerhead design.
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

func init() {
	runtime.LockOSThread() // In case we wish to use OpenGL.
}

func main() {
	watch := stopwatch()
	object, err := scene()
	if err != nil {
		log.Fatalf("creating 3D object: %s", err)
	}
	fmt.Println("created object in", watch())
	useGPU := true
	err = render(object, useGPU)
	if err != nil {
		log.Fatal(err)
	}
}

// scene returns the showerhead object.
func scene() (glbuild.Shader3D, error) {
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

func render(s glbuild.Shader3D, useGPU bool) (err error) {
	err = glbuild.ShortenNames3D(&s, 6)
	if err != nil {
		return fmt.Errorf("shortening shader names: %s", err)
	}
	bb := s.Bounds()
	var sdf gleval.SDF3
	watch := stopwatch()
	if useGPU {
		fmt.Println("using GPU")
		{
			terminate, err := gleval.Init1x1GLFW()
			if err != nil {
				return err
			}
			defer terminate()
		}
		source := new(bytes.Buffer)
		_, err = glbuild.NewDefaultProgrammer().WriteComputeSDF3(source, s)
		if err != nil {
			return err
		}
		sdf, err = gleval.NewComputeGPUSDF3(source, bb)
	} else {
		sdf, err = gleval.NewCPUSDF3(s)
	}

	if err != nil || sdf == nil {
		return fmt.Errorf("instantiating SDF: %s", err)
	}

	fmt.Println("instantiating evaluation SDF took", watch())
	const size = 1 << 12
	renderer, err := glrender.NewOctreeRenderer(sdf, bb.Size().Max()/350, size)
	if err != nil {
		return err
	}

	fp, err := os.Create("showerhead.stl")
	if err != nil {
		return fmt.Errorf("creating file: %s", err)
	}
	watch = stopwatch()
	triangles, err := glrender.RenderAll(renderer)
	if err != nil {
		return fmt.Errorf("rendering triangles: %s", err)
	}
	e := sdf.(interface{ Evaluations() uint64 })
	fmt.Println("evaluated SDF", e.Evaluations(), "times and rendered", len(triangles), "triangles in", watch())
	watch = stopwatch()
	_, err = glrender.WriteBinarySTL(fp, triangles)
	if err != nil {
		return fmt.Errorf("writing STL file: %s", err)
	}
	fmt.Println("wrote STL file in", watch())
	return nil
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

func stopwatch() func() time.Duration {
	start := time.Now()
	return func() time.Duration {
		return time.Since(start)
	}
}
