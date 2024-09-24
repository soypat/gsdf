//go:build !tinygo && cgo

package gleval

import (
	"errors"
	"fmt"
	"io"
	"unsafe"

	"github.com/go-gl/gl/all-core/gl"
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
	prog  glgl.Program
	bb    ms3.Box
	evals uint64
}

func (sdf *SDF3Compute) Bounds() ms3.Box {
	return sdf.bb
}

// Evaluations returns total evaluations performed succesfully during sdf's lifetime.
func (sdf *SDF3Compute) Evaluations() uint64 { return sdf.evals }

func (sdf *SDF3Compute) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	err := computeEvaluate(sdf.prog, pos, dist)
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
	err := computeEvaluate(sdf.prog, pos, dist)
	if err != nil {
		return err
	}
	sdf.evals += uint64(len(pos))
	return nil
}

func computeEvaluate[T ms2.Vec | ms3.Vec](prog glgl.Program, pos []T, dist []float32) error {
	prog.Bind()
	defer prog.Unbind()

	posTex, _, err := loadTexture(pos, 0, glgl.ReadOnly)
	if err != nil {
		return err
	}
	defer posTex.Delete()

	distTex, distCfg, err := loadTexture(dist, 1, glgl.WriteOnly)
	if err != nil {
		return err
	}
	err = prog.RunCompute(len(dist), 1, 1)
	if err != nil {
		return err
	}
	err = glgl.GetImage(dist, distTex, distCfg)
	if err != nil {
		return err
	}
	return nil
}

func loadTexture[T float32 | ms2.Vec | ms3.Vec](slice []T, imageUnit uint32, access glgl.AccessUsage) (glgl.Texture, glgl.TextureImgConfig, error) {
	var zero T
	var format uint32
	var internalFormat int32
	switch unsafe.Sizeof(zero) / 4 {
	case 1: // float32
		format = gl.RED
		internalFormat = gl.R32F
	case 2: // ms2.Vec
		format = gl.RG
		internalFormat = gl.RG32F
	case 3: // ms3.Vec
		format = gl.RGB
		internalFormat = gl.RGBA32F
	default:
		panic(fmt.Sprintf("unsupported type %T", zero))
	}
	posCfg := glgl.TextureImgConfig{
		Type:      glgl.Texture2D,
		Width:     len(slice),
		Height:    1,
		Access:    access,
		MinFilter: gl.NEAREST,
		MagFilter: gl.NEAREST,
		Xtype:     gl.FLOAT,
		ImageUnit: imageUnit,

		// Type specific layout attributes.
		Format:         format,
		InternalFormat: internalFormat,
	}
	tex, err := glgl.NewTextureFromImage(posCfg, slice)
	return tex, posCfg, err
}
