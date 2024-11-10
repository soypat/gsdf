//go:build !tinygo && cgo

package gsdf

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"testing"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms1"
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
	rng     *rand.Rand
}

func (cfg *shaderTestConfig) div3(bounds ms3.Box) (int, int, int) {
	sz := bounds.Size()
	nx, ny, nz := cfg.div(sz.X), cfg.div(sz.Y), cfg.div(sz.Z)
	return nx, ny, nz
}
func (cfg *shaderTestConfig) div2(bounds ms2.Box) (int, int) {
	sz := bounds.Size()
	nx, ny := cfg.div(sz.X), cfg.div(sz.Y)
	return nx, ny
}
func (cfg *shaderTestConfig) div(dim float32) int {
	divs := dim / cfg.testres
	return int(ms1.Clamp(divs, 5, 32))
}

// Since GPU must be run in main thread we need to do some dark arts for GPU code to be code-covered.
func TestMain(m *testing.M) {
	err := testGsdfGPU()
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(0) // Remove after actual tests added. Is here to prevent "[no tests to run]" message.
	os.Exit(m.Run())
}

func testGsdfGPU() error {
	const bufsize = 32 * 32 * 32
	runtime.LockOSThread()
	term, err := gleval.Init1x1GLFW()
	if err != nil {
		log.Fatal(err)
	}
	defer term()
	invoc := glgl.MaxComputeInvocations()
	prog := *glbuild.NewDefaultProgrammer()
	prog.SetComputeInvocations(invoc, 1, 1)
	cfg := &shaderTestConfig{
		posbuf:  make([]ms3.Vec, bufsize),
		posbuf2: make([]ms2.Vec, bufsize),
		distbuf: [2][]float32{make([]float32, bufsize), make([]float32, bufsize)},
		testres: 1. / 3,
		prog:    prog,
		rng:     rand.New(rand.NewSource(1)),
	}
	t := &tb{}
	testPrimitives3D(t, cfg)
	if t.fail {
		return errors.New("primitives3D failed")
	}
	testPrimitives2D(t, cfg)
	if t.fail {
		return errors.New("primitives2D failed")
	}
	testBinOp3D(t, cfg)
	if t.fail {
		return errors.New("op3D failed")
	}
	testRandomUnary3D(t, cfg)
	if t.fail {
		return errors.New("randomUnary3D failed")
	}
	testBinary2D(t, cfg)
	if t.fail {
		return errors.New("binary2D failed")
	}
	return nil
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

func testBinOp3D(t *tb, cfg *shaderTestConfig) {
	unionBin := func(a, b glbuild.Shader3D) glbuild.Shader3D {
		return Union(a, b)
	}
	var BinaryOps = []func(a, b glbuild.Shader3D) glbuild.Shader3D{
		unionBin,
		Difference,
		Intersection,
		Xor,
	}
	var smoothOps = []func(k float32, a, b glbuild.Shader3D) glbuild.Shader3D{
		SmoothUnion,
		SmoothDifference,
		SmoothIntersect,
	}

	s1, _ := NewSphere(1)
	s2, _ := NewBox(1, 0.6, .8, 0.1)
	s2 = Translate(s2, 0.5, 0.7, 0.8)
	for _, op := range BinaryOps {
		result := op(s1, s2)
		testShader3D(t, result, cfg)
	}
	for _, op := range smoothOps {
		result := op(0.1, s1, s2)
		testShader3D(t, result, cfg)
	}
}

func testRandomUnary3D(t *tb, cfg *shaderTestConfig) {
	var UnaryRandomizedOps = []func(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D{
		randomRotation,
		randomShell,
		randomElongate,
		randomRound,
		randomScale,
		randomSymmetry,
		randomTranslate,
		// randomArray, // round() differs from go's math.Round()
	}
	var OtherUnaryRandomizedOps2D3D = []func(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D{
		randomExtrude,
		randomRevolve,
	}
	s2, _ := NewBox(1, 0.61, 0.8, 0.3)
	for _, op := range UnaryRandomizedOps {
		result := op(s2, cfg.rng)
		testShader3D(t, result, cfg)
	}
	s2d := &rect2D{d: ms2.Vec{X: 1, Y: 0.57}}
	for _, op := range OtherUnaryRandomizedOps2D3D {
		result := op(s2d, cfg.rng)
		testShader3D(t, result, cfg)
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

func testBinary2D(t *tb, cfg *shaderTestConfig) {
	union := func(a, b glbuild.Shader2D) glbuild.Shader2D {
		return Union2D(a, b)
	}
	s2, _ := NewRectangle(1, 0.61)
	s1, _ := NewCircle(0.4)
	s1 = Translate2D(s1, 0.45, 1)
	var BinaryOps2D = []func(a, b glbuild.Shader2D) glbuild.Shader2D{
		union,
		Difference2D,
		Intersection2D,
		Xor2D,
	}
	for _, op := range BinaryOps2D {
		result := op(s1, s2)
		testShader2D(t, result, cfg)
	}
}

func testShader3D(t *tb, obj glbuild.Shader3D, cfg *shaderTestConfig) {
	bounds := obj.Bounds()
	invocx, _, _ := cfg.prog.ComputeInvocations()
	nx, ny, nz := cfg.div3(bounds)

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
	nx, ny := cfg.div2(bounds)

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

func randomCircArray2D(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	circleDiv := rng.Intn(16) + 3
	nInst := rng.Intn(circleDiv) + 1
	s, err := CircularArray2D(a, nInst, circleDiv)
	if err != nil {
		panic(err)
	}
	return s
}

func randomRotation(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	var axis ms3.Vec
	for ms3.Norm(axis) < .5 {
		axis = ms3.Vec{X: rng.Float32() * 3, Y: rng.Float32() * 3, Z: rng.Float32() * 3}
	}
	const maxAngle = 3.14159
	var angle float32
	for math32.Abs(angle) < 1e-1 || math32.Abs(angle) > 1 {
		angle = 2 * maxAngle * (rng.Float32() - 0.5)
	}
	a, err := Rotate(a, angle, axis)
	if err != nil {
		panic(err)
	}
	return a
}

func randomShell(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	bb := a.Bounds()
	size := bb.Size()
	maxSize := bb.Size().Max() / 128
	thickness := math32.Min(maxSize, rng.Float32())
	if thickness <= 1e-8 {
		thickness = math32.Min(maxSize, rng.Float32())
	}
	shell := Shell(a, thickness)
	// Cut shell to visualize interior.

	center := bb.Center()
	bb.Max.Y = center.Y

	halfbox, _ := NewBox(size.X*20, size.Y/3, size.Z*20, 0)
	halfbox = Translate(halfbox, 0, size.Y/3, 0)
	return Difference(shell, halfbox)
}

func randomArray(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minDim = 0.1
	const maxRepeat = 8
	nx, ny, nz := rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1
	dx, dy, dz := rng.Float32()+minDim, rng.Float32()+minDim, rng.Float32()+minDim
	s, err := Array(a, dx, dy, dz, nx, ny, nz)
	if err != nil {
		panic(err)
	}
	return s
}

func randomElongate(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minDim = 0.0
	const maxDim = 0.3
	const dim = maxDim - minDim
	dx, dy, dz := dim*rng.Float32()+minDim, dim*rng.Float32()+minDim, dim*rng.Float32()+minDim
	return Elongate(a, dx, dy, dz)
}

func randomRound(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	bb := a.Bounds().Size()
	minround := bb.Min() / 64
	maxround := bb.Min() / 2
	round := minround + (rng.Float32() * (maxround - minround))
	return Offset(a, -round)
}

func randomTranslate(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	var p ms3.Vec
	for ms3.Norm(p) < 0.1 {
		p = ms3.Vec{X: rng.Float32(), Y: rng.Float32(), Z: rng.Float32()}
		p = ms3.Scale((rng.Float32()-0.5)*4, p)
	}

	return Translate(a, p.X, p.Y, p.Z)
}

func randomSymmetry(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	q := rng.Uint32()
	for q&0b111 == 0 {
		q = rng.Uint32()
	}
	x := q&(1<<0) != 0
	y := q&(1<<1) != 0
	z := q&(1<<2) != 0
	return Symmetry(a, x, y, z)
}

func randomScale(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minScale, maxScale = 0.01, 100.
	scale := minScale + rng.Float32()*(maxScale-minScale)
	return Scale(a, scale)
}

func randomExtrude(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minheight, maxHeight = 0.01, 40.
	height := minheight + rng.Float32()*(maxHeight-minheight)
	ex, _ := Extrude(a, height)
	return ex
}

func randomRevolve(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minOff, maxOff = 0, 40.
	off := minOff + rng.Float32()*(maxOff-minOff)
	rev, err := Revolve(a, off)
	if err != nil {
		panic(err)
	}
	return rev
}
