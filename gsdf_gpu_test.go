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
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chewxy/math32"
	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/soypat/glgl/math/ms1"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
)

var failedObj glbuild.Shader3D

type shaderTestConfig struct {
	posbufs  [4][]ms3.Vec
	posbuf2s [4][]ms2.Vec
	distbuf  [4][]float32
	testres  float32
	vp       gleval.VecPool
	prog     glbuild.Programmer
	progbuf  bytes.Buffer
	rng      *rand.Rand
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
	runtime.LockOSThread()
	var exit int
	err := testGsdfGPU()
	if err != nil {
		exit = 1
		log.Println(err)
	}
	if failedObj != nil {
		ui(failedObj, 800, 600)
	}
	runtime.UnlockOSThread()
	os.Exit(m.Run() | exit)
}

func testGsdfGPU() error {
	const bufsize = 32 * 32 * 32
	term, err := gleval.Init1x1GLFW()
	if err != nil {
		log.Fatal(err)
	}
	defer term()
	invoc := glgl.MaxComputeInvocations()
	prog := *glbuild.NewDefaultProgrammer()
	prog.SetComputeInvocations(invoc, 1, 1)
	cfg := &shaderTestConfig{
		testres: 1. / 3,
		prog:    prog,
		rng:     rand.New(rand.NewSource(1)),
	}
	for i := range cfg.posbuf2s {
		cfg.posbuf2s[i] = make([]ms2.Vec, bufsize)
		cfg.posbufs[i] = make([]ms3.Vec, bufsize)
		cfg.distbuf[i] = make([]float32, bufsize)
	}
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
	}
	return nil
}

func testPrimitives3D(t *tb, cfg *shaderTestConfig) {
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
	}
	for _, primitive := range primitives {
		testShader3D(t, primitive, cfg)
	}
}

func testBinOp3D(t *tb, cfg *shaderTestConfig) {
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

func testRandomUnary3D(t *tb, cfg *shaderTestConfig) {
	var UnaryRandomizedOps = []func(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D{
		randomRotation,
		randomShell,
		randomElongate,
		randomRound,
		randomScale,
		randomSymmetry,
		randomTranslate,
		randomArray,
	}
	var OtherUnaryRandomizedOps2D3D = []func(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D{
		randomExtrude,
		randomRevolve,
	}
	s2 := bld.NewBox(1, 0.61, 0.8, 0.3)
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
	poly := bld.NewPolygon([]ms2.Vec{
		{X: 0, Y: 0}, {X: 1, Y: 0.4}, {X: 0.87, Y: 0.8},
	})
	var primitives = []glbuild.Shader2D{
		bld.NewCircle(maxdim),
		bld.NewLine2D(0, 0, dimVec.X, dimVec.Y, thick),
		bld.NewRectangle(dimVec.X, dimVec.Y),
		bld.NewArc(dimVec.X, math.Pi/3, thick),
		bld.NewHexagon(maxdim),
		bld.NewEquilateralTriangle(maxdim),
		poly,
	}
	for _, primitive := range primitives {
		testShader2D(t, primitive, cfg)
	}
}

func testBinary2D(t *tb, cfg *shaderTestConfig) {
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

func testRandomUnary2D(t *tb, cfg *shaderTestConfig) {
	obj := bld.NewRectangle(1, 0.61)
	obj = bld.Translate2D(obj, 2, .3)
	var RandUnary2D = []func(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D{
		randomArray2D, // Not sure why does not work.
		randomCircArray2D,
	}
	for _, op := range RandUnary2D {
		result := op(obj, cfg.rng)
		testShader2D(t, result, cfg)
	}
}

func testShader3D(t *tb, obj glbuild.Shader3D, cfg *shaderTestConfig) {
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
	err = sdfcpu.Evaluate(pos, distCPU, vp)
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
		t.Errorf("%s: %s", name, err)
	}
	err = test_bounds(sdfcpu, vp, cfg)
	if err != nil {
		bf := bld.NewBoundsBoxFrame(obj.Bounds())
		obj = bld.Union(obj, bf)
		name := appendShaderName(nil, obj)
		t.Errorf("%s: %s", name, err)
		failedObj = obj
	}
}

func testShader2D(t *tb, obj glbuild.Shader2D, cfg *shaderTestConfig) {
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
	a = bld.Rotate(a, angle, axis)
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
	shell := bld.Shell(a, thickness)
	// Cut shell to visualize interior.

	center := bb.Center()
	bb.Max.Y = center.Y

	halfbox := bld.NewBox(size.X*20, size.Y/3, size.Z*20, 0)
	halfbox = bld.Translate(halfbox, 0, size.Y/3, 0)
	halfbox = bld.Translate(halfbox, 0, size.Y/3, 0)
	return bld.Difference(shell, halfbox)
}

func randomElongate(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minDim = 0.0
	const maxDim = 0.3
	const dim = maxDim - minDim
	dx, dy, dz := dim*rng.Float32()+minDim, dim*rng.Float32()+minDim, dim*rng.Float32()+minDim
	return bld.Elongate(a, dx, dy, dz)
}

func randomRound(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	bb := a.Bounds().Size()
	minround := bb.Min() / 64
	maxround := bb.Min() / 2
	round := minround + (rng.Float32() * (maxround - minround))
	return bld.Offset(a, -round)
}

func randomTranslate(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	var p ms3.Vec
	for ms3.Norm(p) < 0.1 {
		p = ms3.Vec{X: rng.Float32(), Y: rng.Float32(), Z: rng.Float32()}
		p = ms3.Scale((rng.Float32()-0.5)*4, p)
	}

	return bld.Translate(a, p.X, p.Y, p.Z)
}

func randomSymmetry(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	q := rng.Uint32()
	for q&0b111 == 0 {
		q = rng.Uint32()
	}
	x := q&(1<<0) != 0
	y := q&(1<<1) != 0
	z := q&(1<<2) != 0
	return bld.Symmetry(a, x, y, z)
}

func randomScale(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minScale, maxScale = 0.01, 3
	scale := minScale + rng.Float32()*(maxScale-minScale)
	return bld.Scale(a, scale)
}

func randomExtrude(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minheight, maxHeight = 0.01, 4.
	height := minheight + rng.Float32()*(maxHeight-minheight)
	ex := bld.Extrude(a, height)
	return ex
}

func randomRevolve(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minOff, maxOff float32 = 0, 0
	off := minOff + rng.Float32()*(maxOff-minOff)
	rev := bld.Revolve(a, off)
	return rev
}

func randomCircArray2D(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	circleDiv := rng.Intn(16) + 3
	nInst := rng.Intn(circleDiv) + 1
	s := bld.CircularArray2D(a, nInst, circleDiv)
	return s
}

func randomArray2D(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader2D {
	const minDim = 0.1
	const maxRepeat = 8
	nx, ny := rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1
	dx, dy := rng.Float32()+minDim, rng.Float32()+minDim
	s := bld.Array2D(a, dx, dy, nx, ny)
	return s
}

func randomArray(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
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
	const want = "translate2D(OpUnion2D(arc2D|arc2D))"
	arc := bld.NewArc(1, 1, 0.1)
	arc = bld.Union2D(arc, arc)
	arc = bld.Translate2D(arc, 0.1, 2)
	result := string(appendShaderName(nil, arc))
	if result != want {
		t.Errorf("mismatched result got:\n%s\nwant:\n%s", result, want)
	}
}

func getFnName(fnPtr any) string {
	name := runtime.FuncForPC(reflect.ValueOf(fnPtr).Pointer()).Name()
	idx := strings.LastIndexByte(name, '.')
	return name[idx+1:]
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

func ui(s glbuild.Shader3D, width, height int) error {
	bb := s.Bounds()
	// Initialize GLFW
	window, term, err := startGLFW(width, height)
	if err != nil {
		log.Fatal(err)
	}
	defer term()
	var sdfDecl bytes.Buffer
	programmer := glbuild.NewDefaultProgrammer()
	err = glbuild.ShortenNames3D(&s, 12)
	if err != nil {
		return err
	}

	root, _, _, err := programmer.WriteSDFDecl(&sdfDecl, s)
	if err != nil {
		return err
	}
	// Print OpenGL version
	// // Compile shaders and link program
	prog, err := glgl.CompileProgram(glgl.ShaderSource{
		Vertex: `#version 460
in vec2 aPos;
out vec2 vTexCoord;
void main() {
    vTexCoord = aPos * 0.5 + 0.5;
    gl_Position = vec4(aPos, 0.0, 1.0);
}
` + "\x00",
		Fragment: makeFragSource(root, sdfDecl.String()),
	})
	if err != nil {
		log.Fatal(err)
	}
	prog.Bind()
	// Define a quad covering the screen
	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	vertices := []float32{
		-1.0, -1.0,
		1.0, -1.0,
		-1.0, 1.0,
		-1.0, 1.0,
		1.0, -1.0,
		1.0, 1.0,
	}
	gl.BufferData(gl.ARRAY_BUFFER, 4*len(vertices), gl.Ptr(vertices), gl.STATIC_DRAW)
	camDistUniform, err := prog.UniformLocation("uCamDist\x00")
	if err != nil {
		return err
	}
	resUniform, err := prog.UniformLocation("uResolution\x00")
	if err != nil {
		return err
	}
	yawUniform, err := prog.UniformLocation("uYaw\x00") // gl.GetUniformLocation(program, gl.Str("uResolution\x00"))
	if err != nil {
		return err
	}
	pitchUniform, err := prog.UniformLocation("uPitch\x00")
	if err != nil {
		return err
	}
	// Specify the layout of the vertex data
	posAttrib, err := prog.AttribLocation("aPos\x00")
	if err != nil {
		return err
	}
	gl.EnableVertexAttribArray(posAttrib)
	gl.VertexAttribPointer(posAttrib, 2, gl.FLOAT, false, 0, gl.PtrOffset(0))

	// Enable depth testing
	gl.Enable(gl.DEPTH_TEST)

	// Set up mouse input tracking
	diag := bb.Diagonal()
	minZoom := float64(diag * 0.00001)
	maxZoom := float64(diag * 10)
	var (
		yaw              float64
		pitch            float64
		lastMouseX       float64
		lastMouseY       float64
		camDist          float64 = float64(diag) // initial camera distance
		firstMouseMove           = true
		isMousePressed           = false
		yawSensitivity           = 0.005
		pitchSensitivity         = 0.005
		refresh                  = true
	)

	window.SetCursorPosCallback(func(w *glfw.Window, xpos float64, ypos float64) {
		if !isMousePressed {
			return
		}
		refresh = true
		if firstMouseMove {
			lastMouseX = xpos
			lastMouseY = ypos
			firstMouseMove = false
		}

		deltaX := xpos - lastMouseX
		deltaY := ypos - lastMouseY

		yaw += deltaX * yawSensitivity
		pitch -= deltaY * pitchSensitivity // Invert y-axis

		// Clamp pitch
		pi := math.Pi
		maxPitch := pi/2 - 0.01
		if pitch > maxPitch {
			pitch = maxPitch
		}
		if pitch < -maxPitch {
			pitch = -maxPitch
		}

		lastMouseX = xpos
		lastMouseY = ypos
	})

	window.SetScrollCallback(func(w *glfw.Window, xoff, yoff float64) {
		refresh = true
		camDist -= yoff * (camDist*.1 + .01)
		if camDist < minZoom {
			camDist = minZoom // Minimum zoom level
		}
		if camDist > maxZoom {
			camDist = maxZoom // Maximum zoom level
		}
	})

	window.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		switch button {
		case glfw.MouseButtonLeft:
			refresh = true
			if action == glfw.Press {
				isMousePressed = true
				firstMouseMove = true
				window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
			} else if action == glfw.Release {
				isMousePressed = false
				window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
			}

		}
	})

	// Main render loop
	previousTime := glfw.GetTime()
	for !window.ShouldClose() {
		width, height := window.GetSize()
		currentTime := glfw.GetTime()
		elapsedTime := currentTime - previousTime
		previousTime = currentTime
		_ = elapsedTime
		// Clear the screen
		gl.ClearColor(0.0, 0.0, 0.0, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		// Set uniforms
		prog.Bind()
		// gl.UseProgram(program)
		gl.Uniform1f(camDistUniform, float32(camDist))
		gl.Uniform2f(resUniform, float32(width), float32(height))
		gl.Uniform1f(yawUniform, float32(yaw))
		gl.Uniform1f(pitchUniform, float32(pitch))

		// Draw the quad
		gl.BindVertexArray(vao)
		gl.DrawArrays(gl.TRIANGLES, 0, 6)
		// Swap buffers and poll events
		window.SwapBuffers()

		// Limit frame rate
		for {
			time.Sleep(time.Second / 60)
			glfw.PollEvents()
			if refresh || window.ShouldClose() {
				refresh = false
				break
			}
		}
	}
	return nil
}

func makeFragSource(rootSDFName, sdfDecl string) string {
	var buf bytes.Buffer

	buf.WriteString("#version 460\n")
	buf.WriteString(sdfDecl + "\n")
	// Function to calculate the SDF (Signed Distance Function)
	buf.WriteString("float sdf(vec3 p) {\n\treturn " + rootSDFName + "(p); \n};\n")
	buf.WriteString(`in vec2 vTexCoord;
out vec4 fragColor;


uniform vec2 uResolution;
uniform float uYaw;
uniform float uPitch;



// Function to calculate the normal at a point using central differences
vec3 calcNormal(vec3 pos) {
    const float eps = 0.0001;
    vec2 e = vec2(1.0, -1.0) * 0.5773;
    return normalize(
        e.xyy * sdf(pos + e.xyy * eps) +
        e.yyx * sdf(pos + e.yyx * eps) +
        e.yxy * sdf(pos + e.yxy * eps) +
        e.xxx * sdf(pos + e.xxx * eps)
    );
}

uniform float uCamDist; // Distance from the target. Controlled by mouse scroll (zoom).

void main() {
    vec2 fragCoord = vTexCoord * uResolution;

    // Constants
    const float PI = 3.14159265359;

    // Camera setup
    
    vec3 ta = vec3(0.0, 0.0, 0.0); // Camera target at the origin

    // Use accumulated yaw and pitch
    float yaw = uYaw;
    float pitch = uPitch;

    // Clamp pitch to prevent flipping
    pitch = clamp(pitch, -PI / 2.0 + 0.01, PI / 2.0 - 0.01);

    // Calculate camera direction
    vec3 dir;
    dir.x = cos(pitch) * sin(yaw);
    dir.y = sin(pitch);
    dir.z = cos(pitch) * cos(yaw);

    vec3 ro = ta - dir * uCamDist; // Camera position

    // Camera matrix
    vec3 ww = normalize(ta - ro);                        // Forward vector
    vec3 uu = normalize(cross(ww, vec3(0.0, 1.0, 0.0))); // Right vector
    vec3 vv = cross(uu, ww);                             // Up vector

    // Pixel coordinates
    vec2 p = (2.0 * fragCoord - uResolution) / uResolution.y;

    // Create view ray
    vec3 rd = normalize(p.x * uu + p.y * vv + 1.5 * ww);

    // Ray marching
    const float tmax = 100.0;
    float t = 0.0;
    bool hit = false;
    for (int i = 0; i < 256; i++) {
        vec3 pos = ro + t * rd;
        float h = sdf(pos);
        if (h < 0.0001 || t > tmax) {
            hit = true;
            break;
        }
        t += h;
    }

    // Shading/lighting
    vec3 col = vec3(0.0);
    if (hit) {
        vec3 pos = ro + t * rd;
        vec3 nor = calcNormal(pos);
        float dif = clamp(dot(nor, vec3(0.57703)), 0.0, 1.0);
        float amb = 0.5 + 0.5 * dot(nor, vec3(0.0, 1.0, 0.0));
        col = vec3(0.2, 0.3, 0.4) * amb + vec3(0.8, 0.7, 0.5) * dif;
    }

    // Gamma correction
    col = sqrt(col);

    fragColor = vec4(col, 1.0);
}
`)
	buf.WriteByte(0)
	return buf.String()
}

func startGLFW(width, height int) (window *glfw.Window, term func(), err error) {
	if err := glfw.Init(); err != nil {
		log.Fatalln("Failed to initialize GLFW:", err)
	}

	// Create GLFW window
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 6)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.Resizable, glfw.False)

	window, err = glfw.CreateWindow(width, height, "gsdf 3D Shape Visualizer", nil, nil)
	if err != nil {
		log.Fatalln("Failed to create GLFW window:", err)
	}
	window.MakeContextCurrent()

	// Initialize OpenGL
	if err := gl.Init(); err != nil {
		log.Fatalln("Failed to initialize OpenGL:", err)
	}
	return window, glfw.Terminate, err
}
