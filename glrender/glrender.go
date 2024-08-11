package glrender

import (
	"io"

	"github.com/soypat/glgl/math/ms3"
)

const sqrt3 = 1.73205080757

type Renderer interface {
	ReadTriangles(dst []ms3.Triangle) (n int, err error)
}

// RenderAll reads the full contents of a Renderer and returns the slice read.
// It does not return error on io.EOF, like the io.RenderAll implementation.
func RenderAll(r Renderer) ([]ms3.Triangle, error) {
	const startSize = 4096
	var err error
	var nt int
	result := make([]ms3.Triangle, 0, startSize)
	buf := make([]ms3.Triangle, startSize)
	for {
		nt, err = r.ReadTriangles(buf)
		if err == nil || err == io.EOF {
			result = append(result, buf[:nt]...)
		}
		if err != nil {
			break
		}
	}
	if err == io.EOF {
		return result, nil
	}
	return result, err
}

type ivec struct {
	x int
	y int
	z int
}

func (a ivec) Add(b ivec) ivec      { return ivec{x: a.x + b.x, y: a.y + b.y, z: a.z + b.z} }
func (a ivec) AddScalar(f int) ivec { return ivec{x: a.x + f, y: a.y + f, z: a.z + f} }
func (a ivec) ScaleMul(f int) ivec  { return ivec{x: a.x * f, y: a.y * f, z: a.z * f} }
func (a ivec) ScaleDiv(f int) ivec  { return ivec{x: a.x / f, y: a.y / f, z: a.z / f} }
func (a ivec) Sub(b ivec) ivec      { return ivec{x: a.x - b.x, y: a.y - b.y, z: a.z - b.z} }
func (a ivec) Vec() ms3.Vec         { return ms3.Vec{X: float32(a.x), Y: float32(a.y), Z: float32(a.z)} }

type icube struct {
	ivec
	lvl int
}

func (c icube) size(baseRes float32) float32 {
	dim := 1 << (c.lvl - 1)
	return float32(dim) * baseRes
}

func (c icube) box(origin ms3.Vec, size float32) ms3.Box {
	return ms3.Box{
		Min: ms3.Add(origin, ms3.Scale(size, c.ivec.Vec())),
		Max: ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{2, 2, 2}).Vec())),
	}
}

// corners returns the cube corners.
func (c icube) corners(origin ms3.Vec, size float32) [8]ms3.Vec {
	return [8]ms3.Vec{
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{0, 0, 0}).Vec())),
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{2, 0, 0}).Vec())),
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{2, 2, 0}).Vec())),
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{0, 2, 0}).Vec())),
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{0, 0, 2}).Vec())),
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{2, 0, 2}).Vec())),
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{2, 2, 2}).Vec())),
		ms3.Add(origin, ms3.Scale(size, c.ivec.Add(ivec{0, 2, 2}).Vec())),
	}
}

func (c icube) octree() [8]icube {
	lvl := c.lvl - 1
	s := 1 << lvl
	return [8]icube{
		{ivec: c.Add(ivec{0, 0, 0}), lvl: lvl},
		{ivec: c.Add(ivec{s, 0, 0}), lvl: lvl},
		{ivec: c.Add(ivec{s, s, 0}), lvl: lvl},
		{ivec: c.Add(ivec{0, s, 0}), lvl: lvl},
		{ivec: c.Add(ivec{0, 0, s}), lvl: lvl},
		{ivec: c.Add(ivec{s, 0, s}), lvl: lvl},
		{ivec: c.Add(ivec{s, s, s}), lvl: lvl},
		{ivec: c.Add(ivec{0, s, s}), lvl: lvl},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func aligndown(v, alignto int) int {
	return v &^ (alignto - 1)
}
