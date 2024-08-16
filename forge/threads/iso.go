package threads

import (
	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

// ISO is a standardized thread.
// Pitch is usually the number following the diameter
// i.e: for M16x2 the pitch is 2mm
type ISO struct {
	// D is the thread nominal diameter [mm].
	D float32
	// P is the thread pitch [mm].
	P float32
	// Is external or internal thread. Ext set to true means external thread.
	Ext bool
}

var _ Threader = ISO{} // Compile time check of interface implementation.

func (iso ISO) ThreadParams() Parameters {
	b := basic{D: iso.D, P: iso.P}
	return b.ThreadParams()
}

func (iso ISO) Thread() (glbuild.Shader2D, error) {
	radius := iso.D / 2
	theta := 30.0 * math.Pi / 180.
	h := iso.P / (2.0 * math.Tan(theta))
	rMajor := radius
	r0 := rMajor - (7.0/8.0)*h
	var poly ms2.PolygonBuilder
	if iso.Ext {
		// External threeading.
		rRoot := (iso.P / 8.0) / math.Cos(theta)
		xOfs := (1.0 / 16.0) * iso.P
		poly.AddXY(iso.P, 0)
		poly.AddXY(iso.P, r0+h)
		poly.AddXY(iso.P/2.0, r0).Smooth(rRoot, 5)
		poly.AddXY(xOfs, rMajor)
		poly.AddXY(-xOfs, rMajor)
		poly.AddXY(-iso.P/2.0, r0).Smooth(rRoot, 5)
		poly.AddXY(-iso.P, r0+h)
		poly.AddXY(-iso.P, 0)
	} else {
		// Internal threading.
		rMinor := r0 + (1.0/4.0)*h
		rCrest := (iso.P / 16.0) / math.Cos(theta)
		xOfs := (1.0 / 8.0) * iso.P
		poly.AddXY(iso.P, 0)
		poly.AddXY(iso.P, rMinor)
		poly.AddXY(iso.P/2-xOfs, rMinor)
		poly.AddXY(0, r0+h).Smooth(rCrest, 5)
		poly.AddXY(-iso.P/2+xOfs, rMinor)
		poly.AddXY(-iso.P, rMinor)
		poly.AddXY(-iso.P, 0)
	}
	vertices, err := poly.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	return gsdf.NewPolygon(vertices)
}
