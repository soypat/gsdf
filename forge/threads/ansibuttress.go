package threads

import (
	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

type ANSIButtress struct {
	// D is the thread nominal diameter.
	D float32
	// P is the thread pitch.
	P float32
}

var _ Threader = ANSIButtress{} // Compile time check of interface implementation.

func (butt ANSIButtress) ThreadParams() Parameters {
	return basic{D: butt.D, P: butt.P}.ThreadParams()
}

// ANSIButtressThread returns the 2d profile for an ANSI 45/7 buttress thread.
// https://en.wikipedia.org/wiki/Buttress_thread
// ASME B1.9-1973
// radius is radius of thread. pitch is thread-to-thread distance.
func (ansi ANSIButtress) Thread() (glbuild.Shader2D, error) {
	radius := ansi.D / 2
	t0 := math.Tan(45.0 * math.Pi / 180)
	t1 := math.Tan(7.0 * math.Pi / 180)
	const threadEng = 0.6 // thread engagement

	h0 := ansi.P / (t0 + t1)
	h1 := ((threadEng / 2.0) * ansi.P) + (0.5 * h0)
	hp := ansi.P / 2.0

	var tp ms2.PolygonBuilder
	tp.AddXY(ansi.P, 0)
	tp.AddXY(ansi.P, radius)
	tp.AddXY(hp-((h0-h1)*t1), radius)
	tp.AddXY(t0*h0-hp, radius-h1).Smooth(0.0714*ansi.P, 5)
	tp.AddXY((h0-h1)*t0-hp, radius)
	tp.AddXY(-ansi.P, radius)
	tp.AddXY(-ansi.P, 0)

	verts, err := tp.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	return gsdf.NewPolygon(verts)
}
