package main

import (
	"fmt"
	"log"
	"math"
	"runtime"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/textsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

func init() {
	runtime.LockOSThread()
}

// scene generates the 3D object for rendering.
func scene(bld *gsdf.Builder) (glbuild.Shader3D, error) {
	// We create the cover of Gödel, Escher, Bach: an Eternal Golden Braid.
	var f textsdf.Font
	f.Configure(textsdf.FontConfig{
		RelativeGlyphTolerance: 0.01,
		Builder:                bld,
	})
	err := f.LoadTTFBytes(textsdf.ISO3098TTF())
	if err != nil {
		return nil, err
	}
	G, _ := f.Glyph('G')
	E, _ := f.Glyph('E')
	B, _ := f.Glyph('B')

	bbG := G.Bounds()
	bbE := E.Bounds()
	bbB := B.Bounds()
	fmt.Println(bbG, bbE, bbB)
	// return nil, errors.New("basdasd")
	szG := bbG.Size()
	szE := bbE.Size()
	szB := bbB.Size()

	// Match center between letters.
	G = bld.Translate2D(G, -szG.X/2, -szG.Y/2)
	E = bld.Translate2D(E, -szE.X/2, -szE.Y/2)
	B = bld.Translate2D(B, -szB.X/2, -szB.Y/2)
	const round1 = 0.01
	G = bld.Offset2D(G, -round1)
	E = bld.Offset2D(E, -round1)
	B = bld.Offset2D(B, -round1)

	// GEB size. Scale all letters to match size.
	szz := ms2.MaxElem(szG, ms2.MaxElem(szE, szB)).Max()
	sz := ms2.Vec{X: szz, Y: szz}
	sclG := ms2.DivElem(sz, szG)
	sclE := ms2.DivElem(sz, szE)
	sclB := ms2.DivElem(sz, szB)

	// Create 3D letters.
	L := sz.Max()
	G3 := bld.Extrude(G, L)
	E3 := bld.Extrude(E, L)
	B3 := bld.Extrude(B, L)

	// Non-uniform scaling to fill letter intersections.
	G3 = bld.Transform(G3, ms3.ScalingMat4(ms3.Vec{X: sclG.X, Y: sclG.Y, Z: 1}))
	E3 = bld.Transform(E3, ms3.ScalingMat4(ms3.Vec{X: sclE.X, Y: sclE.Y, Z: 1}))
	B3 = bld.Transform(B3, ms3.ScalingMat4(ms3.Vec{X: sclB.X, Y: sclB.Y, Z: 1}))

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

	GEB2 = bld.Translate(GEB2, 0, GEB2.Bounds().Size().Y*1.5, 0)

	return bld.Union(GEB1, GEB2), bld.Err()
}

func main() {
	var bld gsdf.Builder
	// bld.SetFlags(gsdf.FlagUseShaderBuffers)
	shape, err := scene(&bld)
	shape = bld.Scale(shape, 0.3)
	if err != nil {
		log.Fatal("creating scene:", err)
	}
	fmt.Println("Running UI... compiling text shaders may take a while...")
	err = gsdfaux.UI(shape, gsdfaux.UIConfig{
		Width:  800,
		Height: 600,
	})
	if err != nil {
		log.Fatal("UI:", err)
	}
}
