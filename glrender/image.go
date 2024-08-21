package glrender

import (
	"errors"
	"fmt"
	"image"
	"image/color"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf/gleval"
)

type setImage = interface {
	image.Image
	Set(x, y int, c color.Color)
}

// ImageRendererSDF2 converts 2D SDFs to images.
type ImageRendererSDF2 struct {
	conv func(f float32) color.Color
	pos  []ms2.Vec
	dist []float32
}

// NewImageRendererSDF2 instances a new [ImageRendererSDF2] to render images from 2D SDFs. A nil float->color conversion
// function results in a simple black-white color scheme where black is the interior of the SDF (negative distance).
func NewImageRendererSDF2(evalBufferSize int, conversion func(float32) color.Color) (*ImageRendererSDF2, error) {
	if evalBufferSize <= 64 {
		return nil, errors.New("too small evaluation buffer size")
	}
	if conversion == nil {
		conversion = func(f float32) color.Color {
			switch {
			case math32.IsNaN(f) || math32.IsInf(f, 0):
				return color.RGBA{R: 255, A: 255}
			case f > 0:
				return color.White
			default:
				return color.Black
			}
		}
	}
	ir := &ImageRendererSDF2{
		conv: conversion,
		pos:  make([]ms2.Vec, evalBufferSize),
		dist: make([]float32, evalBufferSize),
	}
	return ir, nil
}

// Render maps the SDF2 to the input Image and renders it. It uses userData as an argument to all [gleval.SDF2.Evaluate] calls.
func (ir *ImageRendererSDF2) Render(sdf gleval.SDF2, img setImage, userData any) error {
	imgBB := img.Bounds()
	dxi := imgBB.Dx()
	dyi := imgBB.Dy()
	if len(ir.dist) < dyi {
		return fmt.Errorf("require evaluation buffer (%d) to be at least of length of image rows (%d)", len(ir.dist), dxi)
	}
	bb := sdf.Bounds()
	sz := bb.Size()
	dx := sz.X / float32(dxi)
	dy := sz.Y / float32(dyi)
	bb.Min = ms2.Add(bb.Min, ms2.Vec{X: dx / 2, Y: dy / 2}) // Offset to center image.
	// Keep track of next index to start reading.
	for i := 0; i < dxi; i++ {
		x := float32(i)*dx + bb.Min.X
		err := ir.renderRow(sdf, i, x, bb.Min.Y, dy, imgBB, img, userData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ir *ImageRendererSDF2) renderRow(sdf gleval.SDF2, row int, x, ymin, dy float32, imgBB image.Rectangle, img setImage, userData any) error {
	dyi := imgBB.Dy()
	for j := 0; j < dyi; j++ {
		y := float32(j)*dy + ymin
		ir.pos[j] = ms2.Vec{X: x, Y: y}
	}
	err := sdf.Evaluate(ir.pos[:dyi], ir.dist[:dyi], userData)
	if err != nil {
		return err
	}
	conv := ir.conv
	for j := 0; j < dyi; j++ {
		d := ir.dist[j]
		img.Set(row+imgBB.Min.X, j+imgBB.Min.Y, conv(d))
	}
	return nil
}
