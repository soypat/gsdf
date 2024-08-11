package threads

import (
	"errors"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

// BoltParms defines the parameters for a bolt.
type BoltParms struct {
	Thread      Threader
	Style       NutStyle // head style "hex" or "knurl"
	Tolerance   float32  // subtract from external thread radius
	TotalLength float32  // threaded length + shank length
	ShankLength float32  // non threaded length
}

// Bolt returns a simple bolt suitable for 3d printing.
func Bolt(k BoltParms) (s glbuild.Shader3D, err error) {
	switch {
	case k.Thread == nil:
		err = errors.New("nil Threader")
	case k.TotalLength < 0:
		err = errors.New("total length < 0")
	case k.ShankLength >= k.TotalLength:
		err = errors.New("shank length must be less than total length")
	case k.ShankLength < 0:
		err = errors.New("shank length < 0")
	case k.Tolerance < 0:
		err = errors.New("tolerance < 0")
	}
	if err != nil {
		return nil, err
	}
	param := k.Thread.ThreadParams()

	// head
	var head glbuild.Shader3D
	hr := param.HexRadius()
	hh := param.HexHeight()
	if hr <= 0 || hh <= 0 {
		return nil, errors.New("bad hex head dimension")
	}
	switch k.Style {
	case NutHex:
		head, _ = HexHead(hr, hh, "b")
	case NutKnurl:
		head, _ = KnurledHead(hr, hh, hr*0.25)
	default:
		return nil, errors.New("unknown style for bolt: " + k.Style.String())
	}

	// shank
	shankLength := k.ShankLength + hh/2
	shankOffset := shankLength / 2
	shank, err := gsdf.NewCylinder(param.Radius, shankLength, hh*0.08)
	if err != nil {
		return nil, err
	}
	shank = gsdf.Translate(shank, 0, 0, shankOffset)

	// external thread
	threadLength := k.TotalLength - k.ShankLength
	if threadLength < 0 {
		threadLength = 0
	}
	var thread glbuild.Shader3D
	if threadLength != 0 {
		thread, err = Screw(threadLength, k.Thread)
		if err != nil {
			return nil, err
		}
		// chamfer the thread
		thread, err = chamferedCylinder(thread, 0, 0.5)
		if err != nil {
			return nil, err
		}
		threadOffset := threadLength/2 + shankLength
		thread = gsdf.Translate(thread, 0, 0, threadOffset)
	}
	return gsdf.Union(gsdf.Union(head, shank), thread), nil
}

// chamferedCylinder intersects a chamfered cylinder with an SDF3.
func chamferedCylinder(s glbuild.Shader3D, kb, kt float32) (glbuild.Shader3D, error) {
	// get the length and radius from the bounding box
	bb := s.Bounds()
	l := bb.Max.Z
	r := bb.Max.X
	var poly ms2.PolygonBuilder
	poly.AddXY(0, -l)
	poly.AddXY(r, -l).Chamfer(r * kb)
	poly.AddXY(r, l).Chamfer(r * kt)
	poly.AddXY(0, l)
	verts, err := poly.AppendVertices(nil)
	if err != nil {
		return nil, err
	}
	s2, err := gsdf.NewPolygon(verts)
	if err != nil {
		return nil, err
	}
	cc, err := gsdf.Revolve(s2, 0)
	if err != nil {
		return nil, err
	}
	return gsdf.Intersection(s, cc), nil
}
