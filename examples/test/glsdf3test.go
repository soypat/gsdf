package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/glrender"
)

func main() {
	_, terminate, err := glgl.InitWithCurrentWindow33(glgl.WindowConfig{
		Title:   "compute",
		Version: [2]int{4, 6},
		Width:   1,
		Height:  1,
	})
	if err != nil {
		log.Fatal("FAIL to start GLFW", err.Error())
	}
	defer terminate()

	err = test_visualizer_generation()
	if err != nil {
		log.Fatal("FAIL generating visualization GLSL: ", err.Error())
	}
	err = test_sdf_gpu_cpu()
	if err != nil {
		log.Fatal("FAIL testing CPU/GPU sdf comparisons: ", err.Error())
	}
	err = test_stl_generation()
	if err != nil {
		log.Fatal("FAIL generating STL: ", err.Error())
	}

	log.Println("PASS")
}

var programmer = glbuild.NewDefaultProgrammer()

func init() {
	runtime.LockOSThread() // For GL.
}

var PremadePrimitives = []glbuild.Shader3D{
	mustShader(gsdf.NewHexagonalPrism(1, 2)),
	mustShader(gsdf.NewSphere(1)),
	mustShader(gsdf.NewBoxFrame(1, 1.2, 2.2, .2)),
	mustShader(gsdf.NewBox(1, 1.2, 2.2, 0.3)),
	mustShader(gsdf.NewTorus(3, .5)), // Negative normal?
	mustShader(gsdf.NewTriangularPrism(1, 2)),
	mustShader(gsdf.NewCylinder(1, 3, .1)),
	mustShader(gsdf.NewCylinder(1, 3, 0)), // Cylinder with no rounding.
	mustShader(threads.Screw(5, threads.ISO{ // Negative normal.
		D:   1,
		P:   0.1,
		Ext: true,
	})),
}
var npt threads.NPT
var _ = npt.SetFromNominal(1.0 / 2.0)

var PremadePrimitives2D = []glbuild.Shader2D{
	mustShader2D(gsdf.NewCircle(1)),
	mustShader2D(gsdf.NewHexagon(1)),
	mustShader2D(gsdf.NewPolygon([]ms2.Vec{
		{X: -1, Y: -1}, {X: -1, Y: 0}, {X: 0.5, Y: 2},
	})),
	mustShader2D(npt.Thread()),
	// mustShader2D(gsdf.NewEllipse(1, 2)), // Ellipse seems to be very sensitive to position.
}
var BinaryOps = []func(a, b glbuild.Shader3D) glbuild.Shader3D{
	gsdf.Union,
	gsdf.Difference,
	gsdf.Intersection,
	gsdf.Xor,
}

var BinaryOps2D = []func(a, b glbuild.Shader2D) glbuild.Shader2D{
	gsdf.Union2D,
	gsdf.Difference2D,
	gsdf.Intersection2D,
	gsdf.Xor2D,
}

var SmoothBinaryOps = []func(a, b glbuild.Shader3D, k float32) glbuild.Shader3D{
	gsdf.SmoothUnion,
	gsdf.SmoothDifference,
	gsdf.SmoothIntersect,
}

var OtherUnaryRandomizedOps = []func(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D{
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

const (
	_ = 1 << (iota * 10)
	kB
	MB
)

func test_sdf_gpu_cpu() error {
	const maxBuf = 16 * 16 * 16
	const nx, ny, nz = 10, 10, 10
	vp := &gleval.VecPool{}
	scratchDistCPU := make([]float32, maxBuf)
	scratchDistGPU := make([]float32, maxBuf)
	scratchDist := make([]float32, maxBuf)
	scratchPos := make([]ms3.Vec, maxBuf)
	scratchPos2 := make([]ms2.Vec, maxBuf)
	for _, primitive := range PremadePrimitives {
		log.Printf("begin evaluating %s", getBaseTypename(primitive))
		bounds := primitive.Bounds()
		pos := appendMeshgrid(scratchPos[:0], bounds, nx, ny, nz)
		distCPU := scratchDistCPU[:len(pos)]
		distGPU := scratchDistGPU[:len(pos)]
		sdfcpu, err := gleval.AssertSDF3(primitive)
		if err != nil {
			return err
		}
		err = sdfcpu.Evaluate(pos, distCPU, vp)
		if err != nil {
			return err
		}
		sdfgpu := makeGPUSDF3(primitive)
		err = sdfgpu.Evaluate(pos, distGPU, nil)
		if err != nil {
			return err
		}
		err = cmpDist(pos, distCPU, distGPU)
		if err != nil {
			description := sprintOpPrimitive(nil, primitive)
			return fmt.Errorf("%s: %s", description, err)
		}
		err = test_bounds(sdfcpu, scratchDist, vp)
		if err != nil {
			description := sprintOpPrimitive(nil, primitive)
			return fmt.Errorf("%s: %s", description, err)
		}
	}

	for _, primitive := range PremadePrimitives2D {
		log.Printf("evaluate 2D %s\n", getBaseTypename(primitive))
		bounds := primitive.Bounds()
		pos := appendMeshgrid2D(scratchPos2[:0], bounds, nx, ny, nz)
		distCPU := scratchDistCPU[:len(pos)]
		distGPU := scratchDistGPU[:len(pos)]
		sdfcpu, err := gleval.AssertSDF2(primitive)
		if err != nil {
			return err
		}
		err = sdfcpu.Evaluate(pos, distCPU, vp)
		if err != nil {
			return err
		}
		sdfgpu := makeGPUSDF2(primitive)
		err = sdfgpu.Evaluate(pos, distGPU, nil)
		if err != nil {
			return err
		}
		err = cmpDist(pos, distCPU, distGPU)
		if err != nil {
			description := sprintOpPrimitive(nil, primitive)
			return fmt.Errorf("%s: %s", description, err)
		}
		// err = test_bounds(sdfcpu, scratchDist, vp)
		// if err != nil {
		// 	description := sprintOpPrimitive(nil, primitive)
		// 	return fmt.Errorf("%s: %s", description, err)
		// }
	}

	for _, op := range BinaryOps {
		log.Printf("begin evaluating %s\n", getFnName(op))
		p1 := PremadePrimitives[0]
		p2 := PremadePrimitives[1]
		obj := op(p1, p2)
		bounds := obj.Bounds()
		pos := appendMeshgrid(scratchPos[:0], bounds, nx, ny, nz)
		distCPU := scratchDistCPU[:len(pos)]
		distGPU := scratchDistGPU[:len(pos)]
		sdfcpu, err := gleval.AssertSDF3(obj)
		if err != nil {
			return err
		}
		err = sdfcpu.Evaluate(pos, distCPU, vp)
		if err != nil {
			return err
		}
		sdfgpu := makeGPUSDF3(obj)
		err = sdfgpu.Evaluate(pos, distGPU, nil)
		if err != nil {
			return err
		}
		err = cmpDist(pos, distCPU, distGPU)
		if err != nil {
			description := sprintOpPrimitive(op, p1, p2)
			return fmt.Errorf("%s: %s", description, err)
		}
		err = test_bounds(sdfcpu, scratchDist, vp)
		if err != nil {
			description := sprintOpPrimitive(op, p1, p2)
			return fmt.Errorf("%s: %s", description, err)
		}
	}

	for _, op := range SmoothBinaryOps {
		log.Printf("begin evaluating %s\n", getFnName(op))
		p1 := PremadePrimitives[3]
		p2 := PremadePrimitives[1]
		obj := op(p1, p2, .1)
		bounds := obj.Bounds()
		pos := appendMeshgrid(scratchPos[:0], bounds, nx, ny, nz)
		distCPU := scratchDistCPU[:len(pos)]
		distGPU := scratchDistGPU[:len(pos)]
		sdfcpu, err := gleval.AssertSDF3(obj)
		if err != nil {
			return err
		}
		err = sdfcpu.Evaluate(pos, distCPU, vp)
		if err != nil {
			return err
		}
		sdfgpu := makeGPUSDF3(obj)
		err = sdfgpu.Evaluate(pos, distGPU, nil)
		if err != nil {
			return err
		}
		err = cmpDist(pos, distCPU, distGPU)
		if err != nil {
			description := sprintOpPrimitive(op, p1, p2)
			return fmt.Errorf("%s: %s", description, err)
		}
		err = test_bounds(sdfcpu, scratchDist, vp)
		if err != nil {
			description := sprintOpPrimitive(op, p1, p2)
			return fmt.Errorf("%s: %s", description, err)
		}
	}
	rng := rand.New(rand.NewSource(1))
	for _, op := range OtherUnaryRandomizedOps {
		log.Printf("begin evaluating %s\n", getFnName(op))
		for i := 0; i < 50; i++ {
			primitive := PremadePrimitives[rng.Intn(len(PremadePrimitives))]
			obj := op(primitive, rng)
			bounds := obj.Bounds()
			pos := appendMeshgrid(scratchPos[:0], bounds, nx, ny, nz)
			distCPU := scratchDistCPU[:len(pos)]
			distGPU := scratchDistGPU[:len(pos)]
			sdfcpu, err := gleval.AssertSDF3(obj)
			if err != nil {
				return err
			}
			err = sdfcpu.Evaluate(pos, distCPU, vp)
			if err != nil {
				return err
			}
			sdfgpu := makeGPUSDF3(obj)
			err = sdfgpu.Evaluate(pos, distGPU, nil)
			if err != nil {
				return err
			}
			err = cmpDist(pos, distCPU, distGPU)
			if err != nil {
				description := sprintOpPrimitive(op, primitive)
				return fmt.Errorf("%d %s: %s", i, description, err)
			}
			// log.Printf("allocated v3=%dMB v2=%dMB f32=%dMB", vp.V3.TotalAlloc()/MB, vp.V2.TotalAlloc()/MB, vp.Float.TotalAlloc()/MB)
			if getBaseTypename(primitive) == "screw" ||
				(getBaseTypename(primitive) == "tri" && getFnName(op) == "randomRotation") {
				log.Println("omit screw unary testbounds checks")
				continue
			}
			err = test_bounds(sdfcpu, scratchDist, vp)
			if err != nil {
				description := sprintOpPrimitive(op, primitive)
				return fmt.Errorf("%s: %s", description, err)
			}
		}
	}

	for _, op := range OtherUnaryRandomizedOps2D3D {
		log.Printf("begin evaluating %s\n", getFnName(op))
		for i := 0; i < 10; i++ {
			primitive := PremadePrimitives2D[rng.Intn(len(PremadePrimitives2D))]
			obj := op(primitive, rng)
			bounds := obj.Bounds()
			pos := appendMeshgrid(scratchPos[:0], bounds, nx, ny, nz)
			distCPU := scratchDistCPU[:len(pos)]
			distGPU := scratchDistGPU[:len(pos)]
			sdfcpu, err := gleval.AssertSDF3(obj)
			if err != nil {
				return err
			}
			err = sdfcpu.Evaluate(pos, distCPU, vp)
			if err != nil {
				return err
			}
			sdfgpu := makeGPUSDF3(obj)
			err = sdfgpu.Evaluate(pos, distGPU, nil)
			if err != nil {
				return err
			}
			err = cmpDist(pos, distCPU, distGPU)
			if err != nil {
				description := sprintOpPrimitive(op, primitive)
				return fmt.Errorf("%s: %s", description, err)
			}
			err = vp.AssertAllReleased()
			if err != nil {
				return err
			}
		}
	}
	log.Printf("Allocated: v3:%s  v2:%s  f32:%s", vp.V3.String(), vp.V2.String(), vp.Float.String())
	log.Printf("PASS CPU vs. GPU comparisons.")
	return nil
}

func test_stl_generation() error {
	const r = 1.0 // 1.01
	const diam = 2 * r
	const filename = "sphere.stl"
	// A larger Octree Positional buffer and a smaller RenderAll triangle buffer cause bug.
	const bufsize = 1 << 12
	obj, _ := gsdf.NewSphere(r)
	sdfgpu := makeGPUSDF3(obj)
	renderer, err := glrender.NewOctreeRenderer(sdfgpu, r/64, bufsize)
	if err != nil {
		return err
	}
	renderStart := time.Now()
	triangles, err := glrender.RenderAll(renderer)
	elapsed := time.Since(renderStart)
	if err != nil {
		return err
	}
	fp, _ := os.Create(filename)
	_, err = glrender.WriteBinarySTL(fp, triangles)
	if err != nil {
		return err
	}
	fp.Close()
	fp, err = os.Open(filename)
	if err != nil {
		return err
	}
	defer fp.Close()
	outTriangles, err := glrender.ReadBinarySTL(fp)
	if err != nil {
		return err
	}
	if len(outTriangles) != len(triangles) {
		return fmt.Errorf("wrote %d triangles, read back %d", len(triangles), len(outTriangles))
	}
	for i, got := range outTriangles {
		want := triangles[i]
		if got != want {
			return fmt.Errorf("triangle %d: got %+v, want %+v", i, got, want)
		}
	}
	log.Printf("wrote+read %d triangles with %d evaluations (rendered in %s)", len(triangles), sdfgpu.Evaluations(), elapsed.String())
	return err
}

func test_visualizer_generation() error {
	var s glbuild.Shader3D
	const r = 0.1 // 1.01
	const boxdim = r / 1.2
	const reps = 3
	const diam = 2 * r
	const filename = "visual.glsl"

	point1, _ := gsdf.NewSphere(r / 32)
	point2, _ := gsdf.NewSphere(r / 33)
	point3, _ := gsdf.NewSphere(r / 35)
	point4, _ := gsdf.NewSphere(r / 38)
	zbox, _ := gsdf.NewBox(r/128, r/128, 10*r, r/256)
	point1 = gsdf.Translate(point1, r, 0, 0)
	s = gsdf.Union(zbox, point1)
	s = gsdf.Union(s, gsdf.Translate(point2, 0, r, 0))
	s = gsdf.Union(s, gsdf.Translate(point3, 0, 0, r))
	s = gsdf.Union(s, gsdf.Translate(point4, r, r, r))
	// A larger Octree Positional buffer and a smaller RenderAll triangle buffer cause bug.
	shape, err := threads.Screw(0.2, threads.ISO{
		D:   0.1,
		P:   0.01,
		Ext: true,
	})
	if err != nil {
		return err
	}
	s = gsdf.Union(s, shape)
	return visualize(s, filename)
}

// visualize facilitates the SDF visualization.
func visualize(sdf glbuild.Shader3D, filename string) error {
	bb := sdf.Bounds()
	envelope, err := gsdf.NewBoundsBoxFrame(bb)
	if err != nil {
		return err
	}
	sdf = gsdf.Union(sdf, envelope)
	fp, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer fp.Close()

	const desiredScale = 2.0
	diag := ms3.Norm(bb.Size())
	sdf = gsdf.Scale(sdf, desiredScale/diag)
	written, err := programmer.WriteFragVisualizerSDF3(fp, sdf)
	if err != nil {
		return err
	}
	stat, err := fp.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()
	if int64(written) != size {
		return fmt.Errorf("written (%d) vs filesize (%d) mismatch", written, size)
	}
	return nil
}

func test_bounds(sdf gleval.SDF3, scratchDist []float32, userData any) (err error) {
	typename := getBaseTypename(sdf)
	shader := sdf.(glbuild.Shader3D)
	shader.ForEachChild(nil, func(userData any, s *glbuild.Shader3D) error {
		ss := *s
		typename += "|" + getBaseTypename(ss)
		ss.ForEachChild(userData, func(userData any, s *glbuild.Shader3D) error {
			typename += "|" + getBaseTypename(*s)
			return nil
		})
		return nil
	})
	if strings.Contains(typename, "randomRotation") {
		fmt.Println("omit rotation testbounds (inexact)")
	}

	var skipNormCheck bool
	skipNormCheck = skipNormCheck || strings.Contains(typename, "torus")
	skipNormCheck = skipNormCheck || strings.Contains(typename, "smoothDiff")
	if skipNormCheck {
		log.Println("omit normal checks for", typename)
	}
	defer func() {
		if err != nil {
			filename := "testboundfail_" + typename + ".glsl"
			log.Println("test_bounds failed: generating visualization aid file", filename)
			visualize(shader, filename)
		}
	}()
	const nxbb, nybb, nzbb = 16, 16, 16
	const ndim = nxbb * nybb * nzbb
	const eps = 1e-2
	if len(scratchDist) < ndim {
		return errors.New("minimum len(scratchDist) not met")
	}
	// Evaluate the
	bb := sdf.Bounds()
	size := bb.Size()
	dist := scratchDist[:ndim]

	// We create adjacent bounding boxes to the bounding box
	// being tested and evaluate the SDF there. We look for following inconsistencies:
	//  - Negative distance, which implies interior of SDF outside the intended bounding box.
	//  - Normals which point towards the original bounding box, which imply a SDF surface outside the bounding box.
	var offs = [3]float32{-1, 0, 1}
	originalPos := meshgrid(bb, nxbb, nybb, nzbb)
	newPos := make([]ms3.Vec, len(originalPos))
	normals := make([]ms3.Vec, len(originalPos))

	// Calculate approximate expected normal directions.
	wantNormals := make([]ms3.Vec, len(originalPos))
	wantNormals = appendMeshgrid(wantNormals[:0], bb.Add(ms3.Scale(-1, bb.Center())), nxbb, nybb, nzbb)
	var normOmitLog sync.Once
	var offsize ms3.Vec
	for _, xo := range offs {
		offsize.X = xo * (size.X + eps)
		for _, yo := range offs {
			offsize.Y = yo * (size.Y + eps)
			for _, zo := range offs {
				offsize.Z = zo * (size.Z + eps)
				if xo == 0 && yo == 0 && zo == 0 {
					continue // Would perform not change to bounding box.
				}
				newBB := bb.Add(offsize)
				// New mesh lies outside of bounding box.
				newPos = appendMeshgrid(newPos[:0], newBB, nxbb, nybb, nzbb)
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
				if skipNormCheck {
					continue
				}
				switch typename {
				case "screw", "torus", "smoothDiff":
					normOmitLog.Do(func() {})
					continue // Skip certain shapes for normal calculation. Bad conditioning?
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

func meshgrid(bounds ms3.Box, nx, ny, nz int) []ms3.Vec {
	return appendMeshgrid(make([]ms3.Vec, 0, nx*ny*nz), bounds, nx, ny, nz)
}

func appendMeshgrid(dst []ms3.Vec, bounds ms3.Box, nx, ny, nz int) []ms3.Vec {
	nxyz := ms3.Vec{X: float32(nx), Y: float32(ny), Z: float32(nz)}
	dxyz := ms3.DivElem(bounds.Size(), nxyz)
	var xyz ms3.Vec
	for k := 0; k < nx; k++ {
		xyz.Z = bounds.Min.Z + dxyz.Z*float32(k)
		for j := 0; j < nx; j++ {
			xyz.Y = bounds.Min.Y + dxyz.Y*float32(j)
			for i := 0; i < nx; i++ {
				xyz.X = bounds.Min.X + dxyz.X*float32(i)
				dst = append(dst, xyz)
			}
		}
	}
	return dst
}

func appendMeshgrid2D(dst []ms2.Vec, bounds ms2.Box, nx, ny, nz int) []ms2.Vec {
	nxyz := ms2.Vec{X: float32(nx), Y: float32(ny)}
	dxyz := ms2.DivElem(bounds.Size(), nxyz)
	var xy ms2.Vec
	for j := 0; j < nx; j++ {
		xy.Y = bounds.Min.Y + dxyz.Y*float32(j)
		for i := 0; i < nx; i++ {
			xy.X = bounds.Min.X + dxyz.X*float32(i)
			dst = append(dst, xy)
		}
	}
	return dst
}

func makeGPUSDF3(s glbuild.Shader3D) *gleval.SDF3Compute {
	if s == nil {
		panic("nil Shader3D")
	}
	var source bytes.Buffer
	n, err := programmer.WriteComputeSDF3(&source, s)
	if err != nil {
		panic(err)
	} else if n != source.Len() {
		panic("bytes written mismatch")
	}
	sdfgpu, err := gleval.NewComputeGPUSDF3(&source, s.Bounds())
	if err != nil {
		panic(err)
	}
	return sdfgpu
}

func makeGPUSDF2(s glbuild.Shader2D) gleval.SDF2 {
	if s == nil {
		panic("nil Shader3D")
	}
	var source bytes.Buffer
	n, err := programmer.WriteComputeSDF2(&source, s)
	if err != nil {
		panic(err)
	} else if n != source.Len() {
		panic("bytes written mismatch")
	}
	sdfgpu, err := gleval.NewComputeGPUSDF2(&source, s.Bounds())
	if err != nil {
		panic(err)
	}
	return sdfgpu
}

func mustShader(s glbuild.Shader3D, err error) glbuild.Shader3D {
	if err != nil || s == nil {
		panic(err.Error())
	}
	return s
}

func mustShader2D(s glbuild.Shader2D, err error) glbuild.Shader2D {
	if err != nil || s == nil {
		panic(err.Error())
	}
	return s
}

func cmpDist[T any](pos []T, dcpu, dgpu []float32) error {
	mismatches := 0
	const tol = 5e-3
	var mismatchErr error
	for i, dg := range dcpu {
		dc := dgpu[i]
		diff := math32.Abs(dg - dc)
		if diff > tol {
			mismatches++
			msg := fmt.Sprintf("mismatch: pos=%+v cpu=%f, gpu=%f (diff=%f)", pos[i], dc, dg, diff)
			if mismatchErr == nil {
				mismatchErr = errors.New("cpu vs. gpu distance mismatch")
			}
			log.Print(msg)
			if mismatches > 8 {
				log.Println("too many mismatches")
				return mismatchErr
			}
		}
	}
	return mismatchErr
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
	a, err := gsdf.Rotate(a, angle, axis)
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
	shell := gsdf.Shell(a, thickness)
	// Cut shell to visualize interior.

	center := bb.Center()
	bb.Max.Y = center.Y

	halfbox, _ := gsdf.NewBox(size.X*20, size.Y/3, size.Z*20, 0)
	halfbox = gsdf.Translate(halfbox, 0, size.Y/3, 0)
	return gsdf.Difference(shell, halfbox)
}

func randomArray(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minDim = 0.1
	const maxRepeat = 8
	nx, ny, nz := rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1, rng.Intn(maxRepeat)+1
	dx, dy, dz := rng.Float32()+minDim, rng.Float32()+minDim, rng.Float32()+minDim
	s, err := gsdf.Array(a, dx, dy, dz, nx, ny, nz)
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
	return gsdf.Elongate(a, dx, dy, dz)
}

func randomRound(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	bb := a.Bounds().Size()
	minround := bb.Min() / 64
	maxround := bb.Min() / 2
	round := minround + (rng.Float32() * (maxround - minround))
	return gsdf.Offset(a, -round)
}

func randomTranslate(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	var p ms3.Vec
	for ms3.Norm(p) < 0.1 {
		p = ms3.Vec{X: rng.Float32(), Y: rng.Float32(), Z: rng.Float32()}
		p = ms3.Scale((rng.Float32()-0.5)*4, p)
	}

	return gsdf.Translate(a, p.X, p.Y, p.Z)
}

func randomSymmetry(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	q := rng.Uint32()
	for q&0b111 == 0 {
		q = rng.Uint32()
	}
	x := q&(1<<0) != 0
	y := q&(1<<1) != 0
	z := q&(1<<2) != 0
	return gsdf.Symmetry(a, x, y, z)
}

func randomScale(a glbuild.Shader3D, rng *rand.Rand) glbuild.Shader3D {
	const minScale, maxScale = 0.01, 100.
	scale := minScale + rng.Float32()*(maxScale-minScale)
	return gsdf.Scale(a, scale)
}

func randomExtrude(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minheight, maxHeight = 0.01, 40.
	height := minheight + rng.Float32()*(maxHeight-minheight)
	ex, _ := gsdf.Extrude(a, height)
	return ex
}

func randomRevolve(a glbuild.Shader2D, rng *rand.Rand) glbuild.Shader3D {
	const minOff, maxOff = 0, 40.
	off := minOff + rng.Float32()*(maxOff-minOff)
	rev, err := gsdf.Revolve(a, off)
	if err != nil {
		panic(err)
	}
	return rev
}

func sprintOpPrimitive(op any, primitives ...any) string {
	var buf strings.Builder
	if op != nil {
		if isFn(op) {
			buf.WriteString(getFnName(op))
		} else {
			buf.WriteString(getBaseTypename(op))
			// buf.WriteString(fmt.Sprintf("%+v", op))
		}
		buf.WriteByte('(')
	}
	for i := range primitives {
		buf.WriteString(getBaseTypename(primitives[i]))
		if i < len(primitives)-1 {
			buf.WriteByte(',')
		}
	}
	if op != nil {
		buf.WriteByte(')')
	}
	return buf.String()
}

func getFnName(fnPtr any) string {
	name := runtime.FuncForPC(reflect.ValueOf(fnPtr).Pointer()).Name()
	idx := strings.LastIndexByte(name, '.')
	return name[idx+1:]
}

func isFn(fnPtr any) bool {
	return reflect.ValueOf(fnPtr).Kind() == reflect.Func
}

func getBaseTypename(a any) string {
	s := fmt.Sprintf("%T", a)
	pointIdx := strings.LastIndexByte(s, '.')
	return s[pointIdx+1:]
}
