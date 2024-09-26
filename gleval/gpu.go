//go:build !tinygo && cgo

package gleval

import (
	"errors"
	"fmt"
	"io"

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
)

var errZeroInvoc = errors.New("zero or negative workgroup invocation size, ComputeConfig must have non-zero InvocX field")

// MaxComputeInvoc returns maximum number of invocations/warps per workgroup on the local GPU. The GL context must be actual.
func MaxComputeInvocations() int {
	var invoc int32
	gl.GetIntegerv(gl.MAX_COMPUTE_WORK_GROUP_INVOCATIONS, &invoc)
	return int(invoc)
}

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
func NewComputeGPUSDF3(glglSourceCode io.Reader, bb ms3.Box, cfg ComputeConfig) (*SDF3Compute, error) {
	if cfg.InvocX < 1 {
		return nil, errZeroInvoc
	}
	combinedSource, err := glgl.ParseCombined(glglSourceCode)
	if err != nil {
		return nil, err
	}
	glprog, err := glgl.CompileProgram(combinedSource)
	if err != nil {
		return nil, errors.New(string(combinedSource.Compute) + "\n" + err.Error())
	}
	sdf := SDF3Compute{
		prog:   glprog,
		bb:     bb,
		invocX: cfg.InvocX,
	}
	return &sdf, nil
}

type SDF3Compute struct {
	prog           glgl.Program
	bb             ms3.Box
	evals          uint64
	alignAuxiliary []ms3.Quat
	invocX         int
}

type ComputeConfig struct {
	// InvocX represents the size of the worker group in warps/invocations as configured in the shader.
	// This is configured in a declaration of the following style in the shader:
	//  layout(local_size_x = <InvocX>, local_size_y = 1, local_size_z = 1) in;
	InvocX int
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
	err = computeEvaluate(aligned, dist, sdf.invocX)
	if err != nil {
		return err
	}
	sdf.evals += uint64(len(pos))
	return nil
}

// NewComputeGPUSDF2 instantiates a [SDF2] that runs on the GPU.
func NewComputeGPUSDF2(glglSourceCode io.Reader, bb ms2.Box, cfg ComputeConfig) (*SDF2Compute, error) {
	if cfg.InvocX < 1 {
		return nil, errZeroInvoc
	}
	combinedSource, err := glgl.ParseCombined(glglSourceCode)
	if err != nil {
		return nil, err
	}
	glprog, err := glgl.CompileProgram(combinedSource)
	if err != nil {
		return nil, errors.New(string(combinedSource.Compute) + "\n" + err.Error())
	}
	sdf := SDF2Compute{
		prog:   glprog,
		bb:     bb,
		invocX: cfg.InvocX,
	}
	return &sdf, nil
}

type SDF2Compute struct {
	prog   glgl.Program
	bb     ms2.Box
	evals  uint64
	invocX int
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
	err = computeEvaluate(pos, dist, sdf.invocX)
	if err != nil {
		return err
	}
	sdf.evals += uint64(len(pos))
	return nil
}
