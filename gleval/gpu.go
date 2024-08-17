//go:build !tinygo && cgo

package gleval

import (
	"errors"
	"io"

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
	prog glgl.Program
	bb   ms3.Box
}

func (sdf *SDF3Compute) Bounds() ms3.Box {
	return sdf.bb
}

func (sdf *SDF3Compute) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf.prog.Bind()
	defer sdf.prog.Unbind()
	posCfg := glgl.TextureImgConfig{
		Type:           glgl.Texture2D,
		Width:          len(pos),
		Height:         1,
		Access:         glgl.ReadOnly,
		Format:         gl.RGB,
		MinFilter:      gl.NEAREST,
		MagFilter:      gl.NEAREST,
		Xtype:          gl.FLOAT,
		InternalFormat: gl.RGBA32F,
		ImageUnit:      0,
	}
	_, err := glgl.NewTextureFromImage(posCfg, pos)
	if err != nil {
		return err
	}
	distCfg := glgl.TextureImgConfig{
		Type:           glgl.Texture2D,
		Width:          len(dist),
		Height:         1,
		Access:         glgl.WriteOnly,
		Format:         gl.RED,
		MinFilter:      gl.NEAREST,
		MagFilter:      gl.NEAREST,
		Xtype:          gl.FLOAT,
		InternalFormat: gl.R32F,
		ImageUnit:      1,
	}

	distTex, err := glgl.NewTextureFromImage(distCfg, dist)
	if err != nil {
		return err
	}
	err = sdf.prog.RunCompute(len(dist), 1, 1)
	if err != nil {
		return err
	}
	err = glgl.GetImage(dist, distTex, distCfg)
	if err != nil {
		return err
	}
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
	prog glgl.Program
	bb   ms2.Box
}

func (sdf *SDF2Compute) Bounds() ms2.Box {
	return sdf.bb
}

func (sdf *SDF2Compute) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf.prog.Bind()
	defer sdf.prog.Unbind()
	posCfg := glgl.TextureImgConfig{
		Type:           glgl.Texture2D,
		Width:          len(pos),
		Height:         1,
		Access:         glgl.ReadOnly,
		Format:         gl.RG,
		MinFilter:      gl.NEAREST,
		MagFilter:      gl.NEAREST,
		Xtype:          gl.FLOAT,
		InternalFormat: gl.RG32F,
		ImageUnit:      0,
	}
	_, err := glgl.NewTextureFromImage(posCfg, pos)
	if err != nil {
		return err
	}
	distCfg := glgl.TextureImgConfig{
		Type:           glgl.Texture2D,
		Width:          len(dist),
		Height:         1,
		Access:         glgl.WriteOnly,
		Format:         gl.RED,
		MinFilter:      gl.NEAREST,
		MagFilter:      gl.NEAREST,
		Xtype:          gl.FLOAT,
		InternalFormat: gl.R32F,
		ImageUnit:      1,
	}

	distTex, err := glgl.NewTextureFromImage(distCfg, dist)
	if err != nil {
		return err
	}
	err = sdf.prog.RunCompute(len(dist), 1, 1)
	if err != nil {
		return err
	}
	err = glgl.GetImage(dist, distTex, distCfg)
	if err != nil {
		return err
	}
	return nil
}
