package glrender

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"testing"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/threads"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
)

var bld gsdf.Builder

func TestDualRender(t *testing.T) {
	const (
		shapeDim = 1.0
		divs     = 4
		res      = shapeDim / divs
	)

	shape := bld.NewSphere(shapeDim)
	shape = makeBolt(t)
	sdf, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		t.Fatal(err)
	}
	var dcr DualContourRenderer
	var vp gleval.VecPool
	err = dcr.Reset(sdf, res, &DualContourLeastSquares{}, &vp)
	if err != nil {
		t.Fatal(err)
	}
	tris, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}
	if len(tris) == 0 {
		t.Fatal("no triangles generated")
	}
	fp, _ := os.Create("dual.stl")
	_, err = WriteBinarySTL(fp, tris)
	if err != nil {
		t.Error(err)
	}
}

func TestMinecraftRender(t *testing.T) {
	const (
		shapeDim  = 1.0
		bbScaling = 1.0
		divs      = 4
		res       = shapeDim / divs
		bbOff     = 0 * -res / 2 //res / 2
	)
	shape := bld.NewSphere(shapeDim)
	shape = glbuild.OverloadShader3DBounds(shape, shape.Bounds())
	sdf, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		t.Fatal(err)
	}
	tris, err := minecraftRender(nil, sdf, res)
	if err != nil {
		t.Fatal(err)
	}
	if len(tris) == 0 {
		t.Fatal("no triangles generated")
	}
	fp, _ := os.Create("mc.stl")
	_, err = WriteBinarySTL(fp, tris)
	if err != nil {
		t.Error(err)
	}
}

func TestSphereMarchingTriangles(t *testing.T) {
	const r = 1.0
	const bufsize = 1 << 12
	obj := bld.NewSphere(r)
	sdf, err := gleval.NewCPUSDF3(obj)
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewOctreeRenderer(sdf, r/65, 1<<12+1)
	if err != nil {
		t.Fatal(err)
	}
	tris := testRenderer(t, renderer, nil)
	const expect = 159284
	if len(tris) != expect {
		t.Errorf("expected %d triangles, got %d (diff=%d)", expect, len(tris), len(tris)-expect)
	}
	fp, _ := os.Create("spheretest.stl")
	WriteBinarySTL(fp, tris)
}

func TestOctree(t *testing.T) {
	const r = 1.0 // 1.01
	// A larger Octree Positional buffer and a smaller RenderAll triangle buffer cause bug.
	const bufsize = 1 << 12
	obj := bld.NewSphere(r)
	sdf, err := gleval.NewCPUSDF3(obj)
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewOctreeRenderer(sdf, r/64, bufsize)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range []float32{r / 4, r / 8, r / 64, r / 37, r / 4.000001, r / 13, r / 3.5} {
		err = renderer.Reset(sdf, res)
		if err != nil {
			t.Fatal(err)
		}
		_ = testRenderer(t, renderer, nil)
	}
}

func testRenderer(t *testing.T, oct Renderer, userData any) []ms3.Triangle {
	triangles, err := RenderAll(oct, userData)
	if err != nil {
		t.Fatal(err)
	} else if len(triangles) == 0 {
		t.Fatal("empty triangles")
	}
	var buf bytes.Buffer
	n, err := WriteBinarySTL(&buf, triangles)
	if err != nil {
		t.Fatal(err)
	}
	if n != buf.Len() {
		t.Errorf("want %d bytes written, got %d", buf.Len(), n)
	}
	outTriangles, err := ReadBinarySTL(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(outTriangles) != len(triangles) {
		t.Errorf("wrote %d triangles, read back %d", len(triangles), len(outTriangles))
	}
	for i, got := range outTriangles {
		want := triangles[i]
		if got != want {
			t.Errorf("triangle %d: got %+v, want %+v", i, got, want)
		}
	}
	return triangles
}

func TestRenderImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	renderer, err := NewImageRendererSDF2(4096, nil)
	if err != nil {
		panic(err)
	}
	s := bld.NewCircle(0.5)
	sdf, err := gleval.NewCPUSDF2(s)
	if err != nil {
		t.Fatal(err)
	}
	err = renderer.Render(sdf, img, nil)
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := os.Create("out.png")
	png.Encode(fp, img)
}

func TestIcube(t *testing.T) {
	const tol = 1e-4
	const LVLs = 4
	const RES = 1.0
	const maxdim = RES * (1 << (LVLs - 1))
	bb := ms3.Box{Max: ms3.Vec{X: maxdim, Y: maxdim, Z: maxdim}}
	topcube, origin, err := makeICube(bb, RES)
	if err != nil {
		t.Error(err)
	}
	if origin != (ms3.Vec{}) {
		t.Error("expected origin at (0,0,0)")
	}
	t.Log("truebb", bb)
	levelsVisual("levels.glsl", topcube, 3, origin, RES)

	subcubes := topcube.octree()
	t.Log("top", topcube.lvl, topcube.lvlIdx(), topcube.box(origin, topcube.size(RES)), topcube.size(RES))
	for i, subcube := range subcubes {
		subsize := subcube.size(RES)
		subbox := subcube.box(origin, subsize)
		size := subbox.Size()
		if math32.Abs(size.Max()-subsize) > tol || math32.Abs(size.Min()-subsize) > tol {
			t.Log("size mismatch", size, subsize)
		}
		t.Log("sub", subcube.lvl, subcube.lvlIdx(), subbox, subsize)
		if (i == 0 || i == 1) && subcube.lvl > 1 {
			subcube = subcube.octree()[1]
			t.Log("subsub", subcube.lvl, subcube.lvlIdx(), subcube.box(origin, subcube.size(RES)), subcube.size(RES))
		}
	}
	levels := topcube.lvl
	_ = levels
}

func levelsVisual(filename string, startCube icube, targetLvl int, origin ms3.Vec, res float32) {
	topBB := startCube.box(origin, startCube.size(res))
	cubes := []icube{startCube}
	i := 0
	for cubes[i].lvl > targetLvl {
		subcubes := cubes[i].octree()
		cubes = append(cubes, subcubes[:]...)
		i++
	}
	cubes = cubes[i:]
	bb := bld.NewBoundsBoxFrame(topBB)
	s := bld.NewSphere(res)
	s = bld.Translate(s, origin.X, origin.Y, origin.Z)
	s = bld.Union(bb, s)
	for _, c := range cubes {
		bb := bld.NewBoundsBoxFrame(c.box(origin, c.size(res)))
		s = bld.Union(s, bb)
	}
	s = bld.Scale(s, 0.5/s.Bounds().Size().Max())
	glbuild.ShortenNames3D(&s, 12)
	prog := glbuild.NewDefaultProgrammer()
	fp, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	_, _, err = prog.WriteShaderToyVisualizerSDF3(fp, s)
	if err != nil {
		panic(err)
	}
}

func makeBolt(t *testing.T) glbuild.Shader3D {
	const L, shank = 8, 3
	threader := threads.ISO{D: 3, P: 0.5, Ext: true}
	M3, err := threads.Bolt(&bld, threads.BoltParams{
		Thread:      threader,
		Style:       threads.NutHex,
		TotalLength: L + shank,
		ShankLength: shank,
	})
	if err != nil {
		t.Fatal(err)
	}
	M3 = bld.Rotate(M3, 2.5*math.Pi/2, ms3.Vec{X: 1, Z: 0.1})
	return M3
}
