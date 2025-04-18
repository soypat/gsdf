package threads

import (
	"errors"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

type NutStyle int

const (
	_ NutStyle = iota
	NutCircular
	NutHex
	NutKnurl
)

func (c NutStyle) String() (str string) {
	switch c {
	case NutCircular:
		str = "circular"
	case NutHex:
		str = "hex"
	case NutKnurl:
		str = "knurl"
	default:
		str = "unknown"
	}
	return str
}

// NutParams defines the parameters for a nut.
type NutParams struct {
	Thread    Threader
	Style     NutStyle
	Tolerance float32 // add to internal thread radius
}

// Nut returns a simple nut suitable for 3d printing.
func Nut(bld *gsdf.Builder, k NutParams) (s glbuild.Shader3D, err error) {
	switch {
	case k.Thread == nil:
		err = errors.New("nil threader")
	case k.Tolerance < 0:
		err = errors.New("tolerance < 0")
	}
	if err != nil {
		return nil, err
	}

	params := k.Thread.ThreadParams()
	// nut body
	var nut glbuild.Shader3D
	nr := params.HexRadius()
	nh := params.HexHeight()
	if nr <= 0 || nh <= 0 {
		return nil, errors.New("bad hex nut dimensions")
	}
	switch k.Style {
	case NutHex:
		nut, err = HexHead(bld, nr, nh, true, true)
	case NutKnurl:
		nut, err = KnurledHead(bld, nr, nh, nr*0.25)
	case NutCircular:
		nut = bld.NewCylinder(nr*1.1, nh, 0)
	default:
		err = errors.New("passed argument NutStyle not defined for Nut")
	}
	if err != nil {
		return nil, err
	}
	// internal thread
	thread, err := Screw(bld, nh*(1+1e-2), k.Thread)
	if err != nil {
		return nil, err
	}
	return bld.Difference(nut, thread), nil

}
