package threads

import (
	"math"
	"testing"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/gleval"
)

var bld gsdf.Builder

func TestScrew(t *testing.T) {
	iso := ISO{ // Negative normal.
		D:   1,
		P:   0.1,
		Ext: true,
	}
	shape, err := iso.Thread(&bld)
	if err != nil {
		t.Fatal(err)
	}
	sdf, err := gleval.AssertSDF2(shape)
	if err != nil {
		t.Fatal(err)
	}
	eval := func(x, y float32) float32 {
		var dist [1]float32
		err = sdf.Evaluate([]ms2.Vec{{X: x, Y: y}}, dist[:], &gleval.VecPool{})
		if err != nil {
			t.Fatal(err)
		}
		return dist[0]
	}
	outside := eval(iso.P/2, iso.D/2)
	if outside < 0 || math.IsNaN(float64(outside)) {
		t.Error("expected out of SDF", outside)
	}
	inside := eval(iso.P/2, iso.D/3)
	if inside > 0 || math.IsNaN(float64(inside)) {
		t.Error("expected inside of SDF", inside)
	}
}
