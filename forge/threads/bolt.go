package threads

import (
	"errors"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

// BoltParams defines the parameters for a bolt.
type BoltParams struct {
	Thread      Threader
	Style       NutStyle // head style "hex" or "knurl"
	Tolerance   float32  // subtract from external thread radius
	TotalLength float32  // threaded length + shank length
	ShankLength float32  // non threaded length
}

// Bolt returns a simple bolt suitable for 3d printing.
func Bolt(k BoltParams) (s glbuild.Shader3D, err error) {
	switch {
	case k.Thread == nil:
		err = errors.New("nil Threader")
	case k.TotalLength < 0:
		err = errors.New("total length < 0")
	case k.ShankLength >= k.TotalLength:
		err = errors.New("shank length must be less than total length")
	case k.ShankLength <= 0:
		err = errors.New("shank length <= 0")
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
		head, _ = HexHead(hr, hh, false, true) // Round top side only.
	case NutKnurl:
		head, _ = KnurledHead(hr, hh, hr*0.25)
	default:
		return nil, errors.New("unknown style for bolt: " + k.Style.String())
	}
	screwLen := k.TotalLength - k.ShankLength
	screw, err := Screw(screwLen, k.Thread)
	if err != nil {
		return nil, err
	}
	shank, err := gsdf.NewCylinder(param.Radius, k.ShankLength, hh*0.08)
	if err != nil {
		return nil, err
	}
	shankOff := k.ShankLength/2 + hh/2
	shank = gsdf.Translate(shank, 0, 0, shankOff)
	screw = gsdf.Translate(screw, 0, 0, shankOff+screwLen/2)
	// Does not work:
	// screw, err = chamferedCylinder(screw, 0, 0.5)
	// if err != nil {
	// 	return nil, err
	// }
	return gsdf.Union(screw, gsdf.SmoothUnion(shank, head, hh*0.12)), nil
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
	verts, err := poly.AppendVecs(nil)
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
