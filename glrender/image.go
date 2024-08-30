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
//
// Below is Inigo Quilez's famous color conversion for debugging SDF's:
//
//	func colorConversion(d float32) color.Color {
//		// d /= bb.Size().Max()/4 // Normalize distances using bounding box dimensions to get consistent visuals.
//		var one = ms3.Vec{1, 1, 1}
//		var c ms3.Vec
//		if d > 0 {
//			c = ms3.Vec{0.9, 0.6, 0.3}
//		} else {
//			c = ms3.Vec{0.65, 0.85, 1.0}
//		}
//		c = ms3.Scale(1-math32.Exp(-6*math32.Abs(d)), c)
//		c = ms3.Scale(0.8+0.2*math32.Cos(150*d), c)
//		max := 1 - ms1.SmoothStep(0, 0.01, math32.Abs(d))
//		c = ms3.InterpElem(c, one, ms3.Vec{max, max, max})
//		return color.RGBA{
//			R: uint8(c.X * 255),
//			G: uint8(c.Y * 255),
//			B: uint8(c.Z * 255),
//			A: 255,
//		}
//	}
func NewImageRendererSDF2(evalBufferSize int, conversion func(float32) color.Color) (*ImageRendererSDF2, error) {
	if evalBufferSize < 4096 {
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
	if len(ir.dist) < dxi {
		return fmt.Errorf("require evaluation buffer (%d) to be at least of length of image rows (%d)", len(ir.dist), dxi)
	}
	bb := sdf.Bounds()
	sz := bb.Size()
	dx := sz.X / float32(dxi)
	dy := sz.Y / float32(dyi)
	bb.Min = ms2.Add(bb.Min, ms2.Vec{X: dx / 2, Y: dy / 2}) // Offset to center image.
	// Keep track of next index to start reading.
	for j := 0; j < dyi; j++ {
		// y is inverted in the image interface, maximum index (maxI, maxJ) represents upper left corner.
		// See [image.At] method so we must invert y here.
		y := bb.Max.Y - float32(j)*dy
		err := ir.renderRow(sdf, j, y, bb.Min.X, dx, imgBB, img, userData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ir *ImageRendererSDF2) renderRow(sdf gleval.SDF2, row int, y, xmin, dx float32, imgBB image.Rectangle, img setImage, userData any) error {
	dxi := imgBB.Dx()
	for i := 0; i < dxi; i++ {
		x := float32(i)*dx + xmin
		ir.pos[i] = ms2.Vec{X: x, Y: y}
	}
	err := sdf.Evaluate(ir.pos[:dxi], ir.dist[:dxi], userData)
	if err != nil {
		return err
	}
	conv := ir.conv
	row += imgBB.Min.Y
	for i := 0; i < dxi; i++ {
		d := ir.dist[i]
		img.Set(imgBB.Min.X+i, row, conv(d))
	}
	return nil
}
