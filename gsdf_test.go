package gsdf

import (
	"bytes"
	"math"
	"testing"

	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/glbuild"
)

var bld Builder

func TestTransformDuplicateBug(t *testing.T) {
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
