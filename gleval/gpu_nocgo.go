//go:build tinygo || !cgo

package gleval

import (
	"errors"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/glbuild"
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

func computeEvaluate[T ms2.Vec | ms3.Vec](pos []T, dist []float32, invocX int, objects []glbuild.ShaderObject) (err error) {
	return errNoCGO
}

func (b *Batcher) runBinop(binopBody string, cfg ComputeConfig, dst, A, B []float32) error {
	return errNoCGO
}
