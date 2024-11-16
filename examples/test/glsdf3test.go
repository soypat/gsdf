package main

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
	"github.com/soypat/gsdf/gsdfaux"
)

func main() {
	start := time.Now()
	err := run()
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		log.Fatalf("FAIL in %s: %s", elapsed, err.Error())
	}
	log.Println("PASS in ", elapsed)
}

func run() error {
	_, terminate, err := glgl.InitWithCurrentWindow33(glgl.WindowConfig{
		Title:   "compute",
		Version: [2]int{4, 6},
		Width:   1,
		Height:  1,
	})
	if err != nil {
		log.Fatal("FAIL to start GLFW", err.Error())
	}
	invoc := glgl.MaxComputeInvocations()
	fmt.Println("invoc size:", invoc)
	programmer.SetComputeInvocations(invoc, 1, 1)
	defer terminate()

	err = test_multidisplacegpu()
	if err != nil {
		return fmt.Errorf("testing multi displace GPU: %w", err)
	}
	err = test_linesgpu()
	if err != nil {
		return fmt.Errorf("testing linesGPU: %w", err)
	}
	err = test_polygongpu()
	if err != nil {
		return fmt.Errorf("testing polygonGPU: %w", err)
	}

	err = test_union2D()
	if err != nil {
		return fmt.Errorf("testing union2D: %w", err)
	}
	err = test_union3D()
	if err != nil {
		return fmt.Errorf("testing union3D: %w", err)
	}

	err = test_visualizer_generation()
	if err != nil {
		return fmt.Errorf("generating visualization GLSL: %w", err)
	}
	err = test_sdf_gpu_cpu()
	if err != nil {
		return fmt.Errorf("testing CPU/GPU sdf comparisons: %w", err)
	}
	err = test_stl_generation()
	if err != nil {
		return fmt.Errorf("generating STL: %w", err)
	}
	return nil
}

var bld = &gsdf.Builder{}
var programmer = glbuild.NewDefaultProgrammer()

func init() {
	runtime.LockOSThread() // For GL.
}

var PremadePrimitives = []glbuild.Shader3D{
	mustShader(threads.Screw(bld, 5, threads.ISO{ // Negative normal.
		D:   1,
		P:   0.1,
		Ext: true,
	})),
}
var npt threads.NPT
var _ = npt.SetFromNominal(1.0 / 2.0)

var PremadePrimitives2D = []glbuild.Shader2D{
	mustShader2D(npt.Thread(bld)),

	// mustShader2D(gsdf.NewEllipse(1, 2)), // Ellipse seems to be very sensitive to position.
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

	for _, primitive := range PremadePrimitives2D {
		log.Printf("evaluate 2D %s\n", getBaseTypename(primitive))
		bounds := primitive.Bounds()
		pos := ms2.AppendGrid(scratchPos2[:0], bounds, nx, ny)
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

	for _, primitive := range PremadePrimitives {
		log.Printf("begin evaluating %s", getBaseTypename(primitive))
		bounds := primitive.Bounds()
		pos := ms3.AppendGrid(scratchPos[:0], bounds, nx, ny, nz)
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
	obj := bld.NewSphere(r)
	sdfgpu := makeGPUSDF3(obj)
	renderer, err := glrender.NewOctreeRenderer(sdfgpu, r/64, bufsize)
	if err != nil {
		return err
	}
	renderStart := time.Now()
	triangles, err := glrender.RenderAll(renderer, nil)
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

	point1 := bld.NewSphere(r / 32)
	point2 := bld.NewSphere(r / 33)
	point3 := bld.NewSphere(r / 35)
	point4 := bld.NewSphere(r / 38)
	zbox := bld.NewBox(r/128, r/128, 10*r, r/256)
	point1 = bld.Translate(point1, r, 0, 0)
	s = bld.Union(zbox, point1)
	s = bld.Union(s, bld.Translate(point2, 0, r, 0))
	s = bld.Union(s, bld.Translate(point3, 0, 0, r))
	s = bld.Union(s, bld.Translate(point4, r, r, r))
	// A larger Octree Positional buffer and a smaller RenderAll triangle buffer cause bug.
	shape, err := threads.Screw(bld, 0.2, threads.ISO{
		D:   0.1,
		P:   0.01,
		Ext: true,
	})
	if err != nil {
		return err
	}
	s = bld.Union(s, shape)
	return visualize(s, filename)
}

func test_union2D() error {
	const Nshapes = 32
	var circles []glbuild.Shader2D
	for i := 0; i < Nshapes; i++ {
		c := bld.NewCircle(.1)
		circles = append(circles, bld.Translate2D(c, rand.Float32(), rand.Float32()))
	}
	union := bld.Union2D(circles...)
	sdfCPU, err := gleval.NewCPUSDF2(union)
	if err != nil {
		return err
	}
	sdfGPU, err := gsdfaux.MakeGPUSDF2(union)
	if err != nil {
		return err
	}
	bb := union.Bounds()
	pos := ms2.AppendGrid(nil, bb, 32, 32)
	distCPU := make([]float32, len(pos))
	distGPU := make([]float32, len(pos))
	err = sdfCPU.Evaluate(pos, distCPU, nil)
	if err != nil {
		return err
	}
	err = sdfGPU.Evaluate(pos, distGPU, nil)
	if err != nil {
		return err
	}
	err = cmpDist(pos, distCPU, distGPU)
	if err != nil {
		return err
	}
	return nil
}

func test_union3D() error {
	const Nshapes = 32
	var spheres []glbuild.Shader3D
	for i := 0; i < Nshapes; i++ {
		c := bld.NewSphere(.1)
		spheres = append(spheres, bld.Translate(c, rand.Float32(), rand.Float32(), rand.Float32()))
	}
	union := bld.Union(spheres...)
	sdfCPU, err := gleval.NewCPUSDF3(union)
	if err != nil {
		return err
	}
	sdfGPU := makeGPUSDF3(union)

	bb := union.Bounds()
	pos := ms3.AppendGrid(nil, bb, 32, 32, 32)
	distCPU := make([]float32, len(pos))
	distGPU := make([]float32, len(pos))
	err = sdfCPU.Evaluate(pos, distCPU, nil)
	if err != nil {
		return err
	}
	err = sdfGPU.Evaluate(pos, distGPU, nil)
	if err != nil {
		return err
	}
	err = cmpDist(pos, distCPU, distGPU)
	if err != nil {
		return err
	}
	return nil
}

func test_polygongpu() error {
	const Nvertices = 13
	var polybuilder ms2.PolygonBuilder
	polybuilder.Nagon(Nvertices, 2)
	vecs, err := polybuilder.AppendVecs(nil)
	if err != nil {
		return err
	}

	poly := bld.NewPolygon(vecs)
	if err != nil {
		return err
	}
	sdfcpu, err := gleval.NewCPUSDF2(poly)
	if err != nil {
		return err
	}
	polyGPU := &gleval.PolygonGPU{Vertices: vecs}
	invocX, _, _ := programmer.ComputeInvocations()
	polyGPU.Configure(gleval.ComputeConfig{InvocX: invocX})
	return testsdf2("poly", sdfcpu, polyGPU)
}

func test_multidisplacegpu() error {
	const Nshapes = 300
	circle := bld.NewCircle(.2)
	var displace []ms2.Vec
	for i := 0; i < Nshapes; i++ {
		displace = append(displace, ms2.Vec{X: 1 + rand.Float32(), Y: 2 * rand.Float32()})
	}
	var sdfs []glbuild.Shader2D
	for _, vv := range displace {
		sdf := bld.Translate2D(circle, vv.X, vv.Y)
		sdfs = append(sdfs, sdf)
	}
	union := sdfs[0]
	if len(sdfs) > 1 {
		union = bld.Union2D(sdfs...)
	}

	displCPU, err := gsdfaux.MakeGPUSDF2(union)
	if err != nil {
		return err
	}
	displGPU := &gleval.DisplaceMulti2D{
		Displacements: displace,
	}
	invocX, _, _ := programmer.ComputeInvocations()
	err = displGPU.Configure(programmer, circle, gleval.ComputeConfig{InvocX: invocX})
	if err != nil {
		return err
	}
	return testsdf2("multidisp", displCPU, displGPU)
}

func test_linesgpu() error {
	const Nlines = 300
	const width = 1

	var lines [][2]ms2.Vec
	for i := 0; i < Nlines; i++ {
		lines = append(lines, [2]ms2.Vec{
			{X: rand.Float32(), Y: rand.Float32()},
			{X: 1 + rand.Float32(), Y: 2 * rand.Float32()},
		})
	}
	var sdfs []glbuild.Shader2D
	for _, vv := range lines {
		line := bld.NewLine2D(vv[0].X, vv[0].Y, vv[1].X, vv[1].Y, width)
		sdfs = append(sdfs, line)
	}
	union := sdfs[0]
	if len(sdfs) > 1 {
		union = bld.Union2D(sdfs...)
	}

	linesCPU, err := gsdfaux.MakeGPUSDF2(union)
	if err != nil {
		return err
	}
	linesGPU := &gleval.Lines2DGPU{
		Lines: lines,
		Width: width,
	}
	invocX, _, _ := programmer.ComputeInvocations()
	linesGPU.Configure(gleval.ComputeConfig{InvocX: invocX})
	return testsdf2("lines", linesCPU, linesGPU)
}

func testsdf2(name string, sdfcpu, sdfgpu gleval.SDF2) (err error) {
	bbGPU := sdfgpu.Bounds()
	bbCPU := sdfcpu.Bounds()
	if !bbGPU.Equal(bbCPU, 1e-8) {
		return fmt.Errorf("bounding boxes not equal diff: Dmax=%v  Dmin=%v", ms2.Sub(bbCPU.Max, bbGPU.Max), ms2.Sub(bbCPU.Min, bbGPU.Min))
	}
	err = gsdfaux.RenderPNGFile(name+"_gpu.png", sdfgpu, 512, nil)
	if err != nil {
		return err
	}
	err = gsdfaux.RenderPNGFile(name+"_cpu.png", sdfcpu, 512, nil)
	if err != nil {
		return err
	}
	var pos []ms2.Vec
	for _, sz := range []int{32, 256, 512} {
		now := time.Now()
		pos = ms2.AppendGrid(pos[:0], bbGPU, sz, sz)
		distCPU := make([]float32, len(pos))
		distGPU := make([]float32, len(pos))
		err = sdfgpu.Evaluate(pos, distGPU, nil)
		if err != nil {
			return err
		}
		gpuElapsed := time.Since(now).Round(time.Millisecond)
		if len(pos) <= 2*math.MaxUint16 {
			// Only check for small case.
			now = time.Now()
			err = sdfcpu.Evaluate(pos, distCPU, nil)
			cpuElapsed := time.Since(now).Round(time.Millisecond)
			if err != nil {
				return err
			}
			err = cmpDist(pos, distCPU, distGPU)
			if err != nil {
				return err
			}
			fmt.Println("\t", name, len(pos), "gpuelased:", gpuElapsed, "cpuelased:", cpuElapsed)
		} else {
			fmt.Println("\t", name, len(pos), "gpuelased:", gpuElapsed)
		}
	}
	log.Println("PASS", name)
	return nil
}

// visualize facilitates the SDF visualization.
func visualize(sdf glbuild.Shader3D, filename string) error {
	bb := sdf.Bounds()
	envelope := bld.NewBoundsBoxFrame(bb)
	sdf = bld.Union(sdf, envelope)
	fp, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer fp.Close()

	const desiredScale = 2.0
	diag := ms3.Norm(bb.Size())
	sdf = bld.Scale(sdf, desiredScale/diag)
	written, ssbos, err := programmer.WriteShaderToyVisualizerSDF3(fp, sdf)
	if err != nil {
		return err
	} else if len(ssbos) > 0 {
		return errors.New("objectsunsupported in frag visualizer")
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
	originalPos := ms3.AppendGrid(nil, bb, nxbb, nybb, nzbb)
	newPos := make([]ms3.Vec, len(originalPos))
	normals := make([]ms3.Vec, len(originalPos))

	// Calculate approximate expected normal directions.
	wantNormals := make([]ms3.Vec, len(originalPos))
	wantNormals = ms3.AppendGrid(wantNormals[:0], bb.Add(ms3.Scale(-1, bb.Center())), nxbb, nybb, nzbb)
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
				newPos = ms3.AppendGrid(newPos[:0], newBB, nxbb, nybb, nzbb)
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

func makeGPUSDF3(s glbuild.Shader3D) *gleval.SDF3Compute {
	if s == nil {
		panic("nil Shader3D")
	}
	var source bytes.Buffer
	n, ssbos, err := programmer.WriteComputeSDF3(&source, s)
	if err != nil {
		panic(err)
	} else if n != source.Len() {
		panic("bytes written mismatch")
	}
	invocX, _, _ := programmer.ComputeInvocations()
	sdfgpu, err := gleval.NewComputeGPUSDF3(&source, s.Bounds(), gleval.ComputeConfig{
		InvocX:        invocX,
		ShaderObjects: ssbos,
	})
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

	n, ssbos, err := programmer.WriteComputeSDF2(&source, s)
	if err != nil {
		panic(err)
	} else if n != source.Len() {
		panic("bytes written mismatch")
	}
	invocX, _, _ := programmer.ComputeInvocations()
	sdfgpu, err := gleval.NewComputeGPUSDF2(&source, s.Bounds(), gleval.ComputeConfig{
		InvocX:        invocX,
		ShaderObjects: ssbos,
	})
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
	for i, dc := range dcpu {
		dg := dgpu[i]
		diff := math32.Abs(dg - dc)
		if diff > tol {
			mismatches++
			msg := fmt.Sprintf("mismatch: pos=%+v cpu=%f, gpu=%f (diff=%f) idx=%d", pos[i], dc, dg, diff, i)
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
