package threads

import (
	"errors"

	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

// Knurled Cylinders
// See: https://en.wikipedia.org/wiki/Knurling
// This code builds a knurl with the intersection of left and right hand
// multistart screw "threads".

// KnurlParams specifies the knurl parameters.
type KnurlParams struct {
	Length float32 // length of cylinder
	Radius float32 // radius of cylinder
	Pitch  float32 // knurl pitch
	Height float32 // knurl height
	Theta  float32 // knurl helix angle
	starts int
}

// Thread implements the Threader interface.
func (k KnurlParams) Thread(bld *gsdf.Builder) (glbuild.Shader2D, error) {
	var knurl ms2.PolygonBuilder
	knurl.AddXY(k.Pitch/2, 0)
	knurl.AddXY(k.Pitch/2, k.Radius)
	knurl.AddXY(0, k.Radius+k.Height)
	knurl.AddXY(-k.Pitch/2, k.Radius)
	knurl.AddXY(-k.Pitch/2, 0)
	//knurl.Render("knurl.dxf")
	verts, err := knurl.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	return bld.NewPolygon(verts), nil
}

// Parameters implements the Threader interface.
func (k KnurlParams) ThreadParams() Parameters {
	p := ISO{D: k.Radius * 2, P: k.Pitch, Ext: true}.ThreadParams()
	p.Starts = k.starts
	return p
}

// Knurl returns a knurled cylinder.
func Knurl(bld *gsdf.Builder, k KnurlParams) (s glbuild.Shader3D, err error) {
	switch {
	case k.Length <= 0:
		return nil, errors.New("zero or negative Knurl length")
	case k.Radius <= 0:
		return nil, errors.New("zero or negative Knurl radius")
	case k.Pitch <= 0:
		return nil, errors.New("zero or negative Knurl pitch")
	case k.Height <= 0:
		return nil, errors.New("zero or negative Knurl height")
	case k.Theta < 0:
		return nil, errors.New("zero Knurl helix angle")
	case k.Theta >= math.Pi/2:
		return nil, errors.New("too large Knurl helix angle")
	}

	// Work out the number of starts using the desired helix angle.
	k.starts = int(2 * math.Pi * k.Radius * math.Tan(k.Theta) / k.Pitch)
	// create the left/right hand spirals
	knurl0_3d, err := Screw(bld, k.Length, k)
	if err != nil {
		return nil, err
	}
	k.starts *= -1
	knurl1_3d, err := Screw(bld, k.Length, k)
	if err != nil {
		return nil, err
	}

	return bld.Intersection(knurl0_3d, knurl1_3d), nil
}

// KnurledHead returns a generic cylindrical knurled head.
func KnurledHead(bld *gsdf.Builder, radius float32, height float32, pitch float32) (s glbuild.Shader3D, err error) {
	cylinderRound := radius * 0.05
	knurlLength := pitch * math.Floor((height-cylinderRound)/pitch)
	k := KnurlParams{
		Length: knurlLength,
		Radius: radius,
		Pitch:  pitch,
		Height: pitch * 0.3,
		Theta:  45.0 * math.Pi / 180,
	}
	knurl, err := Knurl(bld, k)
	if err != nil {
		return s, err
	}

	cylinder := bld.NewCylinder(radius, height, cylinderRound)
	return bld.Union(cylinder, knurl), nil
}
