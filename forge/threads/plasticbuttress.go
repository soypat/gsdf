package threads

import (
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

type PlasticButtress struct {
	// D is the thread nominal diameter.
	D float32
	// P is the thread pitch.
	P float32
}

var _ Threader = PlasticButtress{} // Compile time check of interface implementation.

func (butt PlasticButtress) ThreadParams() Parameters {
	return basic(butt).ThreadParams()
}

// Thread returns the 2d profile for a screw top style plastic buttress thread.
// Similar to ANSI 45/7 - but with more corner rounding
// radius is radius of thread. pitch is thread-to-thread distance.
func (butt PlasticButtress) Thread() (glbuild.Shader2D, error) {
	radius := butt.D / 2
	const (
		t0 = 1.0                // math.Tan(45.0 * math.Pi / 180)
		t1 = 0.1227845609029046 // math.Tan(7.0 * math.Pi / 180)
	)
	const threadEngage = 0.6 // thread engagement

	h0 := butt.P / (t0 + t1)
	h1 := ((threadEngage / 2.0) * butt.P) + (0.5 * h0)
	hp := butt.P / 2.0
	var tp ms2.PolygonBuilder

	tp.AddXY(butt.P, 0)
	tp.AddXY(butt.P, radius)
	tp.AddXY(hp-((h0-h1)*t1), radius).Smooth(0.05*butt.P, 5)
	tp.AddXY(t0*h0-hp, radius-h1).Smooth(0.15*butt.P, 5)
	tp.AddXY((h0-h1)*t0-hp, radius).Smooth(0.15*butt.P, 5)
	tp.AddXY(-butt.P, radius)
	tp.AddXY(-butt.P, 0)
	verts, err := tp.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	return gsdf.NewPolygon(verts)
}
