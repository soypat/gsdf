//go:build !tinygo && cgo

package gleval

import (
	"errors"
	"fmt"
	"io"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
)

// Init1x1GLFW starts a 1x1 sized GLFW so that user can start working with GPU.
// It returns a termination function that should be called when user is done running loads on GPU.
func Init1x1GLFW() (terminate func(), err error) {
	_, terminate, err = glgl.InitWithCurrentWindow33(glgl.WindowConfig{
		Title:   "compute",
		Version: [2]int{4, 6},
		Width:   1,
		Height:  1,
	})
	return terminate, err
}

// NewComputeGPUSDF3 instantiates a [SDF3] that runs on the GPU.
func NewComputeGPUSDF3(glglSourceCode io.Reader, bb ms3.Box) (*SDF3Compute, error) {
	combinedSource, err := glgl.ParseCombined(glglSourceCode)
	if err != nil {
		return nil, err
	}
	glprog, err := glgl.CompileProgram(combinedSource)
	if err != nil {
		return nil, errors.New(string(combinedSource.Compute) + "\n" + err.Error())
	}
	sdf := SDF3Compute{
		prog: glprog,
		bb:   bb,
	}
	return &sdf, nil
}

type SDF3Compute struct {
	prog           glgl.Program
	bb             ms3.Box
	evals          uint64
	alignAuxiliary []ms3.Quat
}

func (sdf *SDF3Compute) Bounds() ms3.Box {
	return sdf.bb
}

// Evaluations returns total evaluations performed succesfully during sdf's lifetime.
func (sdf *SDF3Compute) Evaluations() uint64 { return sdf.evals }

func (sdf *SDF3Compute) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf.prog.Bind()
	defer sdf.prog.Unbind()
	err := glgl.Err()
	if err != nil {
		return fmt.Errorf("binding SDF3Compute program: %w", err)
	}
	if len(sdf.alignAuxiliary) < len(pos) {
		sdf.alignAuxiliary = append(sdf.alignAuxiliary, make([]ms3.Quat, len(pos)-len(sdf.alignAuxiliary))...)
	}
	aligned := sdf.alignAuxiliary[:len(pos)]
	for i := range aligned {
		aligned[i].V = pos[i]
	}
	err = computeEvaluate(sdf.prog, aligned, dist)
	if err != nil {
		return err
	}
	sdf.evals += uint64(len(pos))
	return nil
}

// NewComputeGPUSDF2 instantiates a [SDF2] that runs on the GPU.
func NewComputeGPUSDF2(glglSourceCode io.Reader, bb ms2.Box) (*SDF2Compute, error) {
	combinedSource, err := glgl.ParseCombined(glglSourceCode)
	if err != nil {
		return nil, err
	}
	glprog, err := glgl.CompileProgram(combinedSource)
	if err != nil {
		return nil, errors.New(string(combinedSource.Compute) + "\n" + err.Error())
	}
	sdf := SDF2Compute{
		prog: glprog,
		bb:   bb,
	}
	return &sdf, nil
}

type SDF2Compute struct {
	prog  glgl.Program
	bb    ms2.Box
	evals uint64
}

func (sdf *SDF2Compute) Bounds() ms2.Box {
	return sdf.bb
}

// Evaluations returns total evaluations performed succesfully during sdf's lifetime.
func (sdf *SDF2Compute) Evaluations() uint64 { return sdf.evals }

func (sdf *SDF2Compute) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf.prog.Bind()
	defer sdf.prog.Unbind()
	err := glgl.Err()
	if err != nil {
		return fmt.Errorf("binding SDF2Compute program: %w", err)
	}
	err = computeEvaluate(sdf.prog, pos, dist)
	if err != nil {
		return err
	}
	sdf.evals += uint64(len(pos))
	return nil
}
