//go:build tinygo || !cgo

package gleval

import (
	"errors"
	"io"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
)

var errNoCGO = errors.New("GPU evaluation requires CGo and is not supported on TinyGo")

// Init1x1GLFW starts a 1x1 sized GLFW so that user can start working with GPU.
// It returns a termination function that should be called when user is done running loads on GPU.
func Init1x1GLFW() (terminate func(), err error) {
	return nil, errNoCGO
}

// NewComputeGPUSDF3 instantiates a [SDF3] that runs on the GPU.
func NewComputeGPUSDF3(glglSourceCode io.Reader, bb ms3.Box) (*SDF3Compute, error) {
	return nil, errNoCGO
}

type SDF3Compute struct {
	bb ms3.Box
}

func (sdf *SDF3Compute) Bounds() ms3.Box {
	return sdf.bb
}

func (sdf *SDF3Compute) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	return errNoCGO
}

// NewComputeGPUSDF2 instantiates a [SDF2] that runs on the GPU.
func NewComputeGPUSDF2(glglSourceCode io.Reader, bb ms2.Box) (*SDF2Compute, error) {
	return nil, errNoCGO
}

type SDF2Compute struct {
	bb ms2.Box
}

func (sdf *SDF2Compute) Bounds() ms2.Box {
	return sdf.bb
}

func (sdf *SDF2Compute) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	return errNoCGO
}
