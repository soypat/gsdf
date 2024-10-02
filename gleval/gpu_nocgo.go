//go:build tinygo || !cgo

package gleval

import (
	"errors"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
)

var errNoCGO = errors.New("GPU evaluation requires CGo and is not supported on TinyGo")

func (poly *PolygonGPU) evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	return errNoCGO
}

func (lines *Lines2DGPU) evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	return errNoCGO
}

func (lines *DisplaceMulti2D) evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	return errNoCGO
}

func computeEvaluate[T ms2.Vec | ms3.Quat](pos []T, dist []float32, invocX int) (err error) {
	return errNoCGO
}
