//go:build !tinygo

package gleval

import (
	"errors"
	"io"

	"github.com/go-gl/gl/all-core/gl"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
)

// NewComputeGPUSDF3 instantiates a SDF3 that runs on the GPU.
func NewComputeGPUSDF3(glglSourceCode io.Reader, bb ms3.Box) (SDF3, error) {
	combinedSource, err := glgl.ParseCombined(glglSourceCode)
	if err != nil {
		return nil, err
	}
	glprog, err := glgl.CompileProgram(combinedSource)
	if err != nil {
		return nil, errors.New(string(combinedSource.Compute) + "\n" + err.Error())
	}
	sdf := computeSDF{
		prog: glprog,
		bb:   bb,
	}
	return &sdf, nil
}

type computeSDF struct {
	prog glgl.Program
	bb   ms3.Box
}

func (sdf *computeSDF) Bounds() ms3.Box {
	return sdf.bb
}

func (sdf *computeSDF) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
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
