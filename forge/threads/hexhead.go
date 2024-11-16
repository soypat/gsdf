package threads

import (
	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

// Hex Heads for nuts and bolts.

// HexHead3D returns the rounded hex head for a nut or bolt.
// Ability to round positive side and/or negative side of hex head
// provided. By convention the negative side is the top of the hex in this package.
func HexHead(bld *gsdf.Builder, radius float32, height float32, roundNeg, roundPos bool) (s glbuild.Shader3D, err error) {
	// basic hex body
	cornerRound := radius * 0.08
	var poly ms2.PolygonBuilder
	poly.Nagon(6, radius-cornerRound)
	vertices, err := poly.AppendVecs(nil)
	if err != nil {
		return nil, err
	}
	hex2d := bld.NewPolygon(vertices)
	if err != nil {
		return nil, err
	}
	hex2d = bld.Offset2D(hex2d, -cornerRound)
	hex3d := bld.Extrude(hex2d, height)

	// round out the top and/or bottom as required
	if roundPos || roundNeg {
		topRound := radius * 1.6
		d := radius * cosd30
		sphere := bld.NewSphere(topRound)
		if err != nil {
			return nil, err
		}
		zOfs := math.Sqrt(topRound*topRound-d*d) - height/2
		if roundNeg {
			hex3d = bld.Intersection(hex3d, bld.Translate(sphere, 0, 0, -zOfs))
		}
		if roundPos {
			hex3d = bld.Intersection(hex3d, bld.Translate(sphere, 0, 0, zOfs))
		}
	}
	return hex3d, nil // TODO error handling.
}
