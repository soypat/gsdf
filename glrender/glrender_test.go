package glrender

import (
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
)

func TestRenderImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	renderer, err := NewImageRendererSDF2(4096, nil)
	if err != nil {
		panic(err)
	}
	s, _ := gsdf.NewCircle(0.5)
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
	bb, _ := gsdf.NewBoundsBoxFrame(topBB)
	s, _ := gsdf.NewSphere(res)
	s = gsdf.Translate(s, origin.X, origin.Y, origin.Z)
	s = gsdf.Union(bb, s)
	for _, c := range cubes {
		bb, err := gsdf.NewBoundsBoxFrame(c.box(origin, c.size(res)))
		if err != nil {
			panic(err)
		}
		s = gsdf.Union(s, bb)
	}
	s = gsdf.Scale(s, 0.5/s.Bounds().Size().Max())
	glbuild.ShortenNames3D(&s, 8)
	prog := glbuild.NewDefaultProgrammer()
	fp, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	var ssbo []glbuild.ShaderObject
	_, ssbo, err = prog.WriteShaderToyVisualizerSDF3(fp, s)
	if err != nil {
		panic(err)
	} else if len(ssbo) > 0 {
		panic("unexpected ssbo")
	}
}
