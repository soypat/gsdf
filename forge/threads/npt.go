package threads

import (
	"errors"

	math "github.com/chewxy/math32"
	"github.com/soypat/gsdf/glbuild"
)

type NPT struct {
	// D is the thread nominal diameter.
	D float32
	// threads per inch. 1.0/TPI gives pitch.
	TPI float32
	// Flat-to-flat hex distance.
	// Can be set by SetFromNominal with a standard value.
	F2F float32
}

var _ Threader = NPT{} // Compile time check of interface implementation.

func (npt NPT) ThreadParams() Parameters {
	p := ISO{D: npt.D, P: 1.0 / npt.TPI}.ThreadParams()
	p.Name = "NPT"
	p.Taper = math.Atan(1.0 / 32.0) // standard NPT taper.
	if npt.F2F > 0 {
		p.HexF2F = npt.F2F
	}
	return p
}

func (npt NPT) Thread() (glbuild.Shader2D, error) {
	return ISO{D: npt.D, P: 1.0 / npt.TPI}.Thread()
}

type nptSpec struct {
	N    float32 // Nominal measurement (usually a fraction of inch)
	D    float32 // screw major diameter
	tpi  float32 // threads per inch
	ftof float32 // hex head flat to flat distance
}

var nptLookupTable = []nptSpec{
	{N: 1.0 / 8.0, D: 0.405, tpi: 27, ftof: 11.2 / 25.4},
	{N: 1.0 / 4.0, D: 0.540, tpi: 18, ftof: 15.7 / 25.4},
	{N: 3.0 / 8.0, D: 0.675, tpi: 18, ftof: 17.5 / 25.4},
	{N: 1.0 / 2.0, D: 0.840, tpi: 14, ftof: 22.4 / 25.4},
	{N: 3.0 / 4.0, D: 1.050, tpi: 14, ftof: 26.9 / 25.4},
	{N: 1.0, D: 1.315, tpi: 11.5, ftof: 35.1 / 25.4},
	{N: 1 + 1.0/4.0, D: 1.660, tpi: 11.5, ftof: 44.5 / 25.4},
	{N: 1 + 1.0/2.0, D: 1.900, tpi: 11.5, ftof: 50.8 / 25.4},
	{N: 2, D: 2.375, tpi: 11.5, ftof: 63.5 / 25.4},
	{N: 2 + 1.0/2.0, D: 2.875, tpi: 8, ftof: 76.2 / 25.4},
	{N: 3, D: 3.500, tpi: 8, ftof: 88.9 / 25.4},
	{N: 4, D: 4.500, tpi: 8, ftof: 117.3 / 25.4},
}

// SetFromNominal sets NPT thread dimensions from a nominal measurement
// which usually takes the form of inch fractions. i.e:
//
//	npt.SetFromNominal(1.0/8.0) // sets NPT 1/8
func (npt *NPT) SetFromNominal(nominalDimension float32) error {
	const lookupTol = 1. / 32.
	for _, a := range nptLookupTable {
		if math.Abs(a.N-nominalDimension) < lookupTol {
			npt.D = a.D
			npt.F2F = a.ftof
			npt.TPI = a.tpi
			return nil
		}
	}
	return errors.New("nominal measurement not found")
}
