package threads

import (
	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

// Acme is a trapezoidal thread form. https://en.wikipedia.org/wiki/Trapezoidal_thread_form
type Acme struct {
	// D is the thread nominal diameter.
	D float32
	// P is the thread pitch.
	P float32
}

var _ Threader = Acme{} // Compile time check of interface implementation.

func (acme Acme) ThreadParams() Parameters {
	return basic{D: acme.D, P: acme.P}.ThreadParams()
}

// AcmeThread returns the 2d profile for an acme thread.
// radius is radius of thread. pitch is thread-to-thread distance.
func (acme Acme) Thread() (glbuild.Shader2D, error) {
	radius := acme.D / 2
	h := radius - 0.5*acme.P
	theta := (29.0 / 2.0) * math.Pi / 180.0
	delta := 0.25 * acme.P * math.Tan(theta)
	xOfs0 := 0.25*acme.P - delta
	xOfs1 := 0.25*acme.P + delta

	var poly ms2.PolygonBuilder
	poly.AddXY(radius, 0)
	poly.AddXY(radius, h)
	poly.AddXY(xOfs1, h)
	poly.AddXY(xOfs0, radius)
	poly.AddXY(-xOfs0, radius)
	poly.AddXY(-xOfs1, h)
	poly.AddXY(-radius, h)
	poly.AddXY(-radius, 0)
	verts, err := poly.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	return gsdf.NewPolygon(verts)
}
