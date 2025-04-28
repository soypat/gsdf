package gsdf_test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms1"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

var testGSDFCalled = false

type shaderTestConfig struct {
	bld       *gsdf.Builder
	useGPU    bool
	posbufs   [4][]ms3.Vec
	posbuf2s  [4][]ms2.Vec
	distbuf   [4][]float32
	testres   float32
	vp        gleval.VecPool
	prog      glbuild.Programmer
	progbuf   bytes.Buffer
	rng       *rand.Rand
	failedObj glbuild.Shader3D
}

func newShaderTestConfig() *shaderTestConfig {
	const bufsize = 32 * 32 * 32
	cfg := &shaderTestConfig{
		testres: 1. / 3,
		prog:    *glbuild.NewDefaultProgrammer(),
		rng:     rand.New(rand.NewSource(1)),
		bld:     &gsdf.Builder{},
	}
	for i := range cfg.posbuf2s {
		cfg.posbuf2s[i] = make([]ms2.Vec, bufsize)
		cfg.posbufs[i] = make([]ms3.Vec, bufsize)
		cfg.distbuf[i] = make([]float32, bufsize)
	}
	return cfg
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

func TestGSDF(t *testing.T) {
	cfg := newShaderTestConfig()
	err := testGSDF(cfg)
	if err != nil {
		t.Error(err)
	}
}

func TestTransformDuplicateBug(t *testing.T) {
	var bld gsdf.Builder
	G := bld.NewCircle(1)
	E := bld.NewCircle(1)
	B := bld.NewCircle(1)

	L := float32(1.0)
	G3 := bld.Extrude(G, L)
	E3 := bld.Extrude(E, L)
	B3 := bld.Extrude(B, L)

	// Non-uniform scaling to fill letter intersections.
	G3 = bld.Transform(G3, ms3.ScalingMat4(ms3.Vec{X: 1.2, Y: 1.3, Z: 1}))
	E3 = bld.Transform(E3, ms3.ScalingMat4(ms3.Vec{X: 1.2, Y: 1.3, Z: 1}))
	B3 = bld.Transform(B3, ms3.ScalingMat4(ms3.Vec{X: 1.2, Y: 1.3, Z: 1}))
	const round2 = 0.025
	G3 = bld.Offset(G3, -round2)
	E3 = bld.Offset(E3, -round2)
	B3 = bld.Offset(B3, -round2)

	// Orient letters.
	const deg90 = math.Pi / 2
	GEB1 := bld.Intersection(G3, bld.Rotate(E3, deg90, ms3.Vec{Y: 1}))
	GEB1 = bld.Intersection(GEB1, bld.Rotate(B3, -deg90, ms3.Vec{X: 1}))

	GEB2 := bld.Intersection(E3, bld.Rotate(G3, deg90, ms3.Vec{Y: 1}))
	GEB2 = bld.Intersection(GEB2, bld.Rotate(B3, -deg90, ms3.Vec{X: 1}))

	GEB2 = bld.Translate(GEB2, 0, 0, GEB2.Bounds().Size().Z*1.5)
	shape := bld.Union(GEB1, GEB2)

	err := glbuild.ShortenNames3D(&shape, 12)
	if err != nil {
		t.Fatal(err)
	}
	prog := glbuild.NewDefaultProgrammer()
	prog.SetComputeInvocations(32, 1, 1)
	var buf bytes.Buffer
	n, _, err := prog.WriteComputeSDF3(&buf, shape)
	if err != nil {
		t.Fatal(err)
	} else if n != buf.Len() {
		t.Error("mismatched length")
	}
}

func TestBuilderErrors(t *testing.T) {
	var bld gsdf.Builder
	bld.SetFlags(gsdf.FlagNoDimensionPanic)
	s := bld.NewCircle(-1)
	if s == nil {
		t.Error("expecting non-nil shape")
	}
	if bld.Err() == nil {
		t.Error("expecting error in gsdf.")
	}
	bld.ClearErrors()
	if bld.Err() != nil {
		t.Error("expected builder error to be cleared")
	}
}

func testGSDF(cfg *shaderTestConfig) error {
	if testGSDFCalled {
		return nil
	}
	testGSDFCalled = true

	t := &tb{}
	var tests = []func(*tb, *shaderTestConfig){
		testPrimitives3D,
		testPrimitives2D,
		testBinOp3D,
		testRandomUnary3D,
		testBinary2D,
		testRandomUnary2D,
	}
	for _, test := range tests {
		test(t, cfg)
		if t.fail {
			return fmt.Errorf("%s: test failed", getFnName(test))
		}
		bldErr := cfg.bld.Err()
		if bldErr != nil {
			t.Errorf("%s: got Builder error %q", getFnName(test), bldErr.Error())
			cfg.bld.ClearErrors()
		}
	}
	return nil
}

func testPrimitives3D(t *tb, cfg *shaderTestConfig) {
	bld := cfg.bld
	const maxdim float32 = 1.0
	dimVec := ms3.Vec{X: maxdim, Y: maxdim * 0.47, Z: maxdim * 0.8}
	thick := maxdim / 10
	var primitives = []glbuild.Shader3D{
		bld.NewSphere(1),
		bld.NewBox(dimVec.X, dimVec.Y, dimVec.Z, thick),
		bld.NewBoxFrame(dimVec.X, dimVec.Y, dimVec.Z, thick),
		bld.NewCylinder(dimVec.X, dimVec.Y, 0),
		bld.NewCylinder(dimVec.X, dimVec.Y, thick),
		bld.NewHexagonalPrism(dimVec.X, dimVec.Y),
		bld.NewTorus(dimVec.X, dimVec.Y),
		bld.NewTriangularPrism(1, 0.5),
		// bld.NewBoundsBoxFrame(ms3.NewBox(0, 0, 0, dimVec.X, dimVec.Y, dimVec.Z)),
	}
	for _, primitive := range primitives {
		testShader3D(t, primitive, cfg)
	}
}

func testBinOp3D(t *tb, cfg *shaderTestConfig) {
	bld := cfg.bld
	unionBin := func(a, b glbuild.Shader3D) glbuild.Shader3D {
		return bld.Union(a, b)
	}
	var BinaryOps = []func(a, b glbuild.Shader3D) glbuild.Shader3D{
		unionBin,
		bld.Difference,
		bld.Intersection,
		bld.Xor,
	}
	var smoothOps = []func(k float32, a, b glbuild.Shader3D) glbuild.Shader3D{
		bld.SmoothUnion,
		bld.SmoothDifference,
		bld.SmoothIntersect,
	}

	s1 := bld.NewSphere(1)
	s2 := bld.NewBox(1, 0.6, .8, 0.1)
	s2 = bld.Translate(s2, 0.5, 0.7, 0.8)
	for _, op := range BinaryOps {
		result := op(s1, s2)
		testShader3D(t, result, cfg)
	}
	for _, op := range smoothOps {
		result := op(0.1, s1, s2)
		testShader3D(t, result, cfg)
	}
}

func testRandomUnary2D(t *tb, cfg *shaderTestConfig) {
	bld := cfg.bld
	obj := bld.NewRectangle(1, 0.61)
	obj = bld.Translate2D(obj, 2, .3)
	var RandUnary2D = []func(*gsdf.Builder, glbuild.Shader2D, *rand.Rand) glbuild.Shader2D{
		randomArray2D, // Not sure why does not work.
		randomCircArray2D,
		randomSymmetry2D,
		randomRotation2D,
		randomAnnulus,
		randomOffset2D,
		randomScale2D,
		randomElongate2D,
	}
	for _, op := range RandUnary2D {
		for i := 0; i < 10; i++ {
			result := op(bld, obj, cfg.rng)
			testShader2D(t, result, cfg)
		}
	}
}

func testRandomUnary3D(t *tb, cfg *shaderTestConfig) {
	bld := cfg.bld
	var UnaryRandomizedOps = []func(*gsdf.Builder, glbuild.Shader3D, *rand.Rand) glbuild.Shader3D{
		randomRotation,
		randomShell,
		randomElongate,
		randomRound,
		randomScale,
		randomSymmetry,
		randomTranslate,
		randomArray,
		randomCircArray,
	}
	var OtherUnaryRandomizedOps2D3D = []func(*gsdf.Builder, glbuild.Shader2D, *rand.Rand) glbuild.Shader3D{
		randomExtrude,
		randomRevolve,
	}
	s2 := bld.NewBox(1, 0.61, 0.8, 0.3)
	for _, op := range UnaryRandomizedOps {
		result := op(bld, s2, cfg.rng)
		testShader3D(t, result, cfg)
	}
	s2d := bld.NewRectangle(1, 0.57)
	for _, op := range OtherUnaryRandomizedOps2D3D {
		result := op(bld, s2d, cfg.rng)
		testShader3D(t, result, cfg)
	}
}

func testPrimitives2D(t *tb, cfg *shaderTestConfig) {
	const maxdim float32 = 1.0
	var pbuilder ms2.PolygonBuilder
	pbuilder.Nagon(8, 1)
	vertices, _ := pbuilder.AppendVecs(nil)
	vPrev := vertices[len(vertices)-1]
	var segments [][2]ms2.Vec
	for i := 0; i < len(vertices); i++ {
		segments = append(segments, [2]ms2.Vec{vPrev, vertices[i]})
		vPrev = vertices[i]
	}
	bld := cfg.bld
	dimVec := ms2.Vec{X: maxdim, Y: maxdim * 0.47}
	thick := maxdim / 10

	// Non-SSBO shapes which use dynamic buffers.
	poly := bld.NewPolygon(vertices)
	polySelfClosed := bld.NewPolygon([]ms2.Vec{{X: 0, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}, {X: 0, Y: 0}})

	// Create shapes to test usage of dynamic buffers as SSBOs.
	bld.SetFlags(bld.Flags() | gsdf.FlagUseShaderBuffers)

	polySSBO := bld.NewPolygon(vertices)
	linesSSBO := bld.NewLines2D(segments, 0.1)
	displaceSSBO := bld.TranslateMulti2D(poly, vertices)

	bld.SetFlags(bld.Flags() &^ gsdf.FlagUseShaderBuffers)

	var primitives = []glbuild.Shader2D{
		bld.NewCircle(maxdim),
		bld.NewLine2D(0, 0, dimVec.X, dimVec.Y, thick),
		bld.NewRectangle(dimVec.X, dimVec.Y),
		bld.NewArc(dimVec.X, math.Pi/3, thick),
		bld.NewHexagon(maxdim),
		bld.NewEquilateralTriangle(maxdim),
		bld.NewEllipse(1, 2),
		poly,
		polySelfClosed,
		polySSBO,
		linesSSBO,
		displaceSSBO,
		bld.NewOctagon(dimVec.X),
		bld.NewDiamond2D(dimVec.X, dimVec.Y),
		bld.NewRoundedX(dimVec.X, thick),
		bld.NewQuadraticBezier2D(dimVec, ms2.Add(dimVec, ms2.Vec{X: maxdim}), ms2.Add(dimVec, ms2.Vec{Y: maxdim}), thick),
	}
	for _, primitive := range primitives {
		testShader2D(t, primitive, cfg)
	}
	// Now test shapes that share shader functions to check that deduplication is working.
	primitives = []glbuild.Shader2D{
		bld.Union2D(
			bld.NewLine2D(1, 2, 3, 4, 0.5),
			bld.NewLine2D(2, 3, 0, 0, 0.2),
			bld.NewLine2D(2, 3, 4, 5, 0.2),
			bld.NewLines2D([][2]ms2.Vec{
				{{X: 0, Y: 0}, {X: 1, Y: 1}},
				{{X: 2, Y: 2}, {X: 3, Y: 1}},
			}, 0.5),
		),
	}
	for _, primitive := range primitives {
		testShader2D(t, primitive, cfg)
	}
}

func testBinary2D(t *tb, cfg *shaderTestConfig) {
	bld := cfg.bld
	union := func(a, b glbuild.Shader2D) glbuild.Shader2D {
		return bld.Union2D(a, b)
	}
	s2 := bld.NewRectangle(1, 0.61)
	s1 := bld.NewCircle(0.4)
	s1 = bld.Translate2D(s1, 0.45, 1)
	var BinaryOps2D = []func(a, b glbuild.Shader2D) glbuild.Shader2D{
		union,
		bld.Difference2D,
		bld.Intersection2D,
		bld.Xor2D,
	}
	for _, op := range BinaryOps2D {
		result := op(s1, s2)
		testShader2D(t, result, cfg)
	}
}

func testShader3D(t *tb, obj glbuild.Shader3D, cfg *shaderTestConfig) {
	bld := cfg.bld
	vp := &cfg.vp
	bounds := obj.Bounds()
	invocx, _, _ := cfg.prog.ComputeInvocations()
	nx, ny, nz := cfg.div3(bounds)

	pos := ms3.AppendGrid(cfg.posbufs[0][:0], bounds, nx, ny, nz)
	distCPU := cfg.distbuf[0][:len(pos)]
	distGPU := cfg.distbuf[1][:len(pos)]

	// Do CPU evaluation.
	sdfcpu, err := gleval.AssertSDF3(obj)
	if err != nil {
		t.Fatal(err)
	}
	err = test_bounds(sdfcpu, vp, cfg)
	if err != nil {
		bf := bld.NewBoundsBoxFrame(obj.Bounds())
		obj = bld.Union(obj, bf)
		name := appendShaderName(nil, obj)
		t.Errorf("%s: %s", name, err)
		cfg.failedObj = obj
	}

	cfg.progbuf.Reset()
	n, objs, err := cfg.prog.WriteComputeSDF3(&cfg.progbuf, obj)
	if err != nil {
		t.Fatal(err)
	}
	if n != cfg.progbuf.Len() {
		t.Fatalf("written bytes not match length of buffer %d != %d", n, cfg.progbuf.Len())
	}
	if !cfg.useGPU {
		return // No GPU usage permitted, nothing else to do.
	}
	// Get CPU positional evaluations.
	err = sdfcpu.Evaluate(pos, distCPU, vp)
	if err != nil {
		t.Fatal(err)
	}

	// Do GPU evaluation.
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
		t.Errorf("%s: %s", name, err)
	}
}

func testShader2D(t *tb, obj glbuild.Shader2D, cfg *shaderTestConfig) {
	failed := t.fail
	defer func() {
		if t.fail && !failed {
			name := "testfail_" + glbuild.SprintShader(obj)
			sdf, _ := gsdfaux.MakeGPUSDF2(obj)
			gsdfaux.RenderPNGFile(name+"_gpu2d.png", sdf, 500, nil)
			sdf, _ = gleval.NewCPUSDF2(obj)
			gsdfaux.RenderPNGFile(name+"_cpu2d.png", sdf, 500, nil)
		}
	}()
	bounds := obj.Bounds()
	invocx, _, _ := cfg.prog.ComputeInvocations()
	nx, ny := cfg.div2(bounds)

	pos := ms2.AppendGrid(cfg.posbuf2s[0][:0], bounds, nx, ny)
	distCPU := cfg.distbuf[0][:len(pos)]
	distGPU := cfg.distbuf[1][:len(pos)]

	// Do CPU evaluation.
	sdfcpu, err := gleval.AssertSDF2(obj)
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
	if !cfg.useGPU {
		return // No GPU usage permitted, end run here.
	}

	err = sdfcpu.Evaluate(pos, distCPU, &cfg.vp)
	if err != nil {
		t.Fatal(err)
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
		t.Errorf("%s: %s", name, err)
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

func randomRotation(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	var axis ms3.Vec
	for ms3.Norm(axis) < .5 {
		axis = ms3.Vec{X: rng.Float32() * 3, Y: rng.Float32() * 3, Z: rng.Float32() * 3}
	}
	const maxAngle = 3.14159
	var angle float32
	for math32.Abs(angle) < 1e-1 || math32.Abs(angle) > 1 {
		angle = 2 * maxAngle * (rng.Float32() - 0.5)
	}
	a = bld.Rotate(a, angle, axis)
	return a
}

func randomShell(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	bb := a.Bounds()
	size := bb.Size()
	maxSize := bb.Size().Max() / 128
	thickness := math32.Min(maxSize, rng.Float32())
	if thickness <= 1e-8 {
		thickness = math32.Min(maxSize, rng.Float32())
	}
	shell := bld.Shell(a, thickness)
	// Cut shell to visualize interior.

	center := bb.Center()
	bb.Max.Y = center.Y

	halfbox := bld.NewBox(size.X*20, size.Y/3, size.Z*20, 0)
	halfbox = bld.Translate(halfbox, 0, size.Y/3, 0)
	halfbox = bld.Translate(halfbox, 0, size.Y/3, 0)
	return bld.Difference(shell, halfbox)
}

func randomElongate(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minDim = 0.0
	const maxDim = 0.3
	const dim = maxDim - minDim
	dx, dy, dz := dim*rng.Float32()+minDim, dim*rng.Float32()+minDim, dim*rng.Float32()+minDim
	return bld.Elongate(a, dx, dy, dz)
}

func randomRound(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	bb := a.Bounds().Size()
	minround := bb.Min() / 64
	maxround := bb.Min() / 2
	round := minround + (rng.Float32() * (maxround - minround))
	return bld.Offset(a, -round)
}

func randomTranslate(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	var p ms3.Vec
	for ms3.Norm(p) < 0.1 {
		p = ms3.Vec{X: rng.Float32(), Y: rng.Float32(), Z: rng.Float32()}
		p = ms3.Scale((rng.Float32()-0.5)*4, p)
	}

	return bld.Translate(a, p.X, p.Y, p.Z)
}

func randomSymmetry(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	q := rng.Uint32()
	for q&0b111 == 0 {
		q = rng.Uint32()
	}
	x := q&(1<<0) != 0
	y := q&(1<<1) != 0
	z := q&(1<<2) != 0
	return bld.Symmetry(a, x, y, z)
}

func randomScale(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minScale, maxScale = 0.01, 3
	scale := minScale + rng.Float32()*(maxScale-minScale)
	return bld.Scale(a, scale)
}

func randomExtrude(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minheight, maxHeight = 0.01, 4.
	height := minheight + rng.Float32()*(maxHeight-minheight)
	ex := bld.Extrude(a, height)
	return ex
}

func randomRevolve(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minOff, maxOff float32 = 0, 0
	off := minOff + rng.Float32()*(maxOff-minOff)
	rev := bld.Revolve(a, off)
	return rev
}

func randomCircArray(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	circleDiv := rng.Intn(16) + 3
	nInst := rng.Intn(circleDiv) + 1
	s := bld.CircularArray(a, nInst, circleDiv)
	return s
}

func randomCircArray2D(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	circleDiv := rng.Intn(16) + 3
	nInst := rng.Intn(circleDiv) + 1
	s := bld.CircularArray2D(a, nInst, circleDiv)
	return s
}

func randomAnnulus(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	s := bld.Annulus(a, rng.Float32())
	return s
}

func randomScale2D(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	s := bld.Scale2D(a, rng.Float32())
	return s
}

func randomElongate2D(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	s := bld.Elongate2D(a, rng.Float32(), rng.Float32())
	return s
}

func randomArray2D(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	const minDim = 0.1
	const maxRepeat = 8
	nx, ny := rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1
	dx, dy := rng.Float32()+minDim, rng.Float32()+minDim
	s := bld.Array2D(a, dx, dy, nx, ny)
	return s
}

func randomSymmetry2D(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	q := rng.Uint32()
	for q&0b111 == 0 {
		q = rng.Uint32()
	}
	return bld.Symmetry2D(a, q&1 != 0, q&2 != 0)
}

func randomOffset2D(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	off := rng.Float32() - 0.5
	return bld.Offset2D(a, off)
}

func randomRotation2D(bld *gsdf.Builder, a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	angle := (math.Pi*rng.Float32() + 0.001)
	return bld.Rotate2D(a, angle)
}

func randomArray(bld *gsdf.Builder, a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minDim = 0.1
	const maxRepeat = 8
	nx, ny, nz := rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1
	dx, dy, dz := rng.Float32()+minDim, rng.Float32()+minDim, rng.Float32()+minDim
	s := bld.Array(a, dx, dy, dz, nx, ny, nz)
	return s
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
	tpname := reflect.TypeOf(obj).String()
	name = append(name, tpname[strings.IndexByte(tpname, '.')+1:]...)
	if len(children) > 0 {
		name = append(name, '(')
		for i := range children {
			name = appendShaderName(name, children[i])
			name = append(name, '|')
		}
		name[len(name)-1] = ')'
	}
	return name
}

func TestAppendShaderName(t *testing.T) {
	var bld gsdf.Builder
	const want = "translate2D(OpUnion2D(arc2D|arc2D))"
	arc := bld.NewArc(1, 1, 0.1)
	arc = bld.Union2D(arc, arc)
	arc = bld.Translate2D(arc, 0.1, 2)
	result := string(appendShaderName(nil, arc))
	if result != want {
		t.Errorf("mismatched result got:\n%s\nwant:\n%s", result, want)
	}
}

func test_bounds(sdf gleval.SDF3, userData any, cfg *shaderTestConfig) (err error) {
	const eps = 1e-2
	// Evaluate the
	bb := sdf.Bounds()
	size := bb.Size()
	nx, ny, nz := cfg.div3(bb)
	// We create adjacent bounding boxes to the bounding box
	// being tested and evaluate the SDF there. We look for following inconsistencies:
	//  - Negative distance, which implies interior of SDF outside the intended bounding box.
	//  - Normals which point towards the original bounding box, which imply a SDF surface outside the bounding box.
	var offs = [3]float32{-1, 0, 1}
	N := nx * ny * nz

	dist := cfg.distbuf[0][:N]
	newPos := cfg.posbufs[1][:N]
	normals := cfg.posbufs[2][:N]
	wantNormals := cfg.posbufs[3][:N]
	// Calculate approximate expected normal directions.
	wantNormals = ms3.AppendGrid(wantNormals[:0], bb.Add(ms3.Scale(-1, bb.Center())), nx, ny, nz)

	var offsize ms3.Vec
	for _, xo := range offs {
		offsize.X = xo * (size.X + eps)
		for _, yo := range offs {
			offsize.Y = yo * (size.Y + eps)
			for _, zo := range offs {
				offsize.Z = zo * (size.Z + eps)
				if xo == 0 && yo == 0 && zo == 0 {
					continue // Would perform no change to bounding box.
				}
				newBB := bb.Add(offsize)
				// New mesh lies outside of bounding box.
				newPos = ms3.AppendGrid(newPos[:0], newBB, nx, ny, nz)
				// Calculate expected normal directions.

				err = sdf.Evaluate(newPos, dist, userData)
				if err != nil {
					return err
				}
				for i, d := range dist {
					if d < 0 {
						return fmt.Errorf("ext bounding box point %v (d=%f) within SDF off=%+v", newPos[i], d, offsize)
					}
				}
				err = gleval.NormalsCentralDiff(sdf, newPos, normals, eps/2, userData)
				if err != nil {
					return err
				}
				for i, got := range normals {
					want := ms3.Add(offsize, wantNormals[i])
					got = ms3.Unit(got)
					angle := ms3.Cos(got, want)
					if angle < math32.Sqrt2/2 {
						msg := fmt.Sprintf("bad norm angle %frad p=%v got %v, want %v -> off=%v bb=%+v", angle, newPos[i], got, want, offsize, newBB)
						if angle <= 0 {
							err = errors.New(msg)
							return err //errors.New(msg) // Definitely have a surface outside of the bounding box.
						} else {
							// fmt.Println("WARN bad normal:", msg) // Is this possible with a surface contained within the bounding box? Maybe an ill-conditioned/pointy surface?
						}
					}
				}
			}
		}
	}
	return nil
}

func getFnName(fnPtr any) string {
	name := runtime.FuncForPC(reflect.ValueOf(fnPtr).Pointer()).Name()
	idx := strings.LastIndexByte(name, '.')
	return name[idx+1:]
}
