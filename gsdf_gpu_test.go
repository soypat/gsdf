//go:build !tinygo && cgo

package gsdf

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"testing"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
)

type shaderTestConfig struct {
	posbuf  []ms3.Vec
	posbuf2 []ms2.Vec
	distbuf [2][]float32
	testres float32
	vp      gleval.VecPool
	prog    glbuild.Programmer
	progbuf bytes.Buffer
}

// Since GPU must be run in main thread we need to do some dark arts for GPU code to be code-covered.
func TestMain(m *testing.M) {
	const bufsize = 32 * 32 * 32
	runtime.LockOSThread()
	term, err := gleval.Init1x1GLFW()
	if err != nil {
		log.Fatal(err)
	}
	invoc := glgl.MaxComputeInvocations()
	prog := *glbuild.NewDefaultProgrammer()
	prog.SetComputeInvocations(invoc, 1, 1)
	cfg := &shaderTestConfig{
		posbuf:  make([]ms3.Vec, bufsize),
		posbuf2: make([]ms2.Vec, bufsize),
		distbuf: [2][]float32{make([]float32, bufsize), make([]float32, bufsize)},
		testres: 1. / 3,
		prog:    prog,
	}
	t := &tb{}
	testPrimitives3D(t, cfg)
	if t.fail {
		os.Exit(1)
	}
	testPrimitives2D(t, cfg)
	if t.fail {
		os.Exit(1)
	}
	os.Exit(0) // Remove after actual tests added. Is here to prevent "[no tests to run]" message.
	exit := func() int {
		defer term()
		return m.Run()
	}()
	os.Exit(exit)
}

func testPrimitives3D(t *tb, cfg *shaderTestConfig) {
	const maxdim float32 = 1.0
	dimVec := ms3.Vec{X: maxdim, Y: maxdim * 0.47, Z: maxdim * 0.8}
	thick := maxdim / 10
	var primitives = []glbuild.Shader3D{
		&sphere{r: 1},
		&box{dims: dimVec, round: thick},
		&cylinder{r: dimVec.X, h: dimVec.Y, round: thick},
		&hex{side: dimVec.X, h: dimVec.Y},
		&torus{rGreater: dimVec.X, rLesser: dimVec.Y},
		&boxframe{dims: dimVec, e: thick},
	}
	for _, primitive := range primitives {
		testShader3D(t, primitive, cfg)
	}
}

func testPrimitives2D(t *tb, cfg *shaderTestConfig) {
	const maxdim float32 = 1.0
	dimVec := ms2.Vec{X: maxdim, Y: maxdim * 0.47}
	thick := maxdim / 10
	var primitives = []glbuild.Shader2D{
		&circle2D{r: maxdim},
		&line2D{width: thick, b: dimVec},
		&rect2D{d: dimVec},
		&arc2D{radius: dimVec.X, angle: math.Pi / 3, thick: thick},
		&hex2D{side: maxdim},
		&equilateralTri2d{hTri: maxdim},
	}
	for _, primitive := range primitives {
		testShader2D(t, primitive, cfg)
	}
}

func testShader3D(t *tb, obj glbuild.Shader3D, cfg *shaderTestConfig) {
	bounds := obj.Bounds()
	invocx, _, _ := cfg.prog.ComputeInvocations()
	sz := bounds.Size()
	nx, ny, nz := int(math32.Max(sz.X/cfg.testres, 3))+1, int(math32.Max(sz.Y/cfg.testres, 3))+1, int(math32.Max(sz.Z/cfg.testres, 3))+1

	pos := ms3.AppendGrid(cfg.posbuf[:0], bounds, nx, ny, nz)
	distCPU := cfg.distbuf[0][:len(pos)]
	distGPU := cfg.distbuf[1][:len(pos)]

	// Do CPU evaluation.
	sdfcpu, err := gleval.AssertSDF3(obj)
	if err != nil {
		t.Fatal(err)
	}
	err = sdfcpu.Evaluate(pos, distCPU, &cfg.vp)
	if err != nil {
		t.Fatal(err)
	}

	// Do GPU evaluation.
	cfg.progbuf.Reset()
	n, objs, err := cfg.prog.WriteComputeSDF3(&cfg.progbuf, obj)
	if err != nil {
		t.Fatal(err)
	}
	if n != cfg.progbuf.Len() {
		t.Fatalf("written bytes not match length of buffer %d != %d", n, cfg.progbuf.Len())
	}
	sdfgpu, err := gleval.NewComputeGPUSDF3(&cfg.progbuf, bounds, gleval.ComputeConfig{
		InvocX:        invocx,
		ShaderObjects: objs,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sdfgpu.Evaluate(pos, distGPU, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = cmpDist(t, pos, distCPU, distGPU)
	if err != nil {
		name := appendShaderName(nil, obj)
		t.Fatalf("%s: %s", name, err)
	}
}

func testShader2D(t *tb, obj glbuild.Shader2D, cfg *shaderTestConfig) {
	bounds := obj.Bounds()
	invocx, _, _ := cfg.prog.ComputeInvocations()
	sz := bounds.Size()
	nx, ny := int(math32.Max(sz.X/cfg.testres, 3))+1, int(math32.Max(sz.Y/cfg.testres, 3))+1

	pos := ms2.AppendGrid(cfg.posbuf2[:0], bounds, nx, ny)
	distCPU := cfg.distbuf[0][:len(pos)]
	distGPU := cfg.distbuf[1][:len(pos)]

	// Do CPU evaluation.
	sdfcpu, err := gleval.AssertSDF2(obj)
	if err != nil {
		t.Fatal(err)
	}
	err = sdfcpu.Evaluate(pos, distCPU, &cfg.vp)
	if err != nil {
		t.Fatal(err)
	}

	// Do GPU evaluation.
	cfg.progbuf.Reset()
	n, objs, err := cfg.prog.WriteComputeSDF2(&cfg.progbuf, obj)
	if err != nil {
		t.Fatal(err)
	}
	if n != cfg.progbuf.Len() {
		t.Fatalf("written bytes not match length of buffer %d != %d", n, cfg.progbuf.Len())
	}
	sdfgpu, err := gleval.NewComputeGPUSDF2(&cfg.progbuf, bounds, gleval.ComputeConfig{
		InvocX:        invocx,
		ShaderObjects: objs,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sdfgpu.Evaluate(pos, distGPU, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = cmpDist(t, pos, distCPU, distGPU)
	if err != nil {
		name := appendShaderName(nil, obj)
		t.Fatalf("%s: %s", name, err)
	}
}

func cmpDist[T any](t *tb, pos []T, dcpu, dgpu []float32) error {
	mismatches := 0
	const tol = 5e-3
	var mismatchErr error
	for i, dc := range dcpu {
		dg := dgpu[i]
		diff := math32.Abs(dg - dc)
		if diff > tol {
			mismatches++
			t.Errorf("mismatch: pos=%+v cpu=%f, gpu=%f (diff=%f) idx=%d", pos[i], dc, dg, diff, i)
			if mismatches > 8 {
				return errors.New("too many mismatched")
			}
		}
	}
	return mismatchErr
}

func appendShaderName(name []byte, obj glbuild.Shader) []byte {
	var children []glbuild.Shader
	if obj3, ok := obj.(glbuild.Shader3D); ok {
		obj3.ForEachChild(nil, func(userData any, s *glbuild.Shader3D) error {
			children = append(children, *s)
			return nil
		})
	} else if obj2, ok := obj.(glbuild.Shader2D); ok {
		obj2.ForEach2DChild(nil, func(userData any, s *glbuild.Shader2D) error {
			children = append(children, *s)
			return nil
		})
	} else {
		panic(fmt.Sprintf("bad object type: %T, with name %s", obj, string(obj.AppendShaderName(nil))))
	}

	name = obj.AppendShaderName(name)
	if len(children) > 0 {
		name = append(name, '(')
		for i := range children {
			appendShaderName(name, children[i])
			name = children[i].AppendShaderName(name)
			name = append(name, '|')
		}
		name[len(name)-1] = ')'
	}
	return name
}

type tb struct {
	fail bool
}

func (t *tb) Error(args ...any) {
	t.fail = true
	log.Print(args...)
}
func (t *tb) Errorf(msg string, args ...any) {
	t.fail = true
	log.Printf(msg, args...)
}

func (t *tb) Fatal(args ...any) {
	t.fail = true
	log.Fatal(args...)
}
func (t *tb) Fatalf(msg string, args ...any) {
	t.fail = true
	log.Fatalf(msg, args...)
}
