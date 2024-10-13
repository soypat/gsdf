package glrender

import (
	"io"

	"github.com/soypat/glgl/math/ms3"
)

const sqrt3 = 1.73205080757

type Renderer interface {
	ReadTriangles(dst []ms3.Triangle, userData any) (n int, err error)
}

// RenderAll reads the full contents of a Renderer and returns the slice read.
// It does not return error on io.EOF, like the io.RenderAll implementation.
func RenderAll(r Renderer, userData any) ([]ms3.Triangle, error) {
	const startSize = 4096
	var err error
	var nt int
	result := make([]ms3.Triangle, 0, startSize)
	buf := make([]ms3.Triangle, startSize)
	for {
		nt, err = r.ReadTriangles(buf, userData)
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

func (a ivec) Add(b ivec) ivec         { return ivec{x: a.x + b.x, y: a.y + b.y, z: a.z + b.z} }
func (a ivec) AddScalar(f int) ivec    { return ivec{x: a.x + f, y: a.y + f, z: a.z + f} }
func (a ivec) MulScalar(f int) ivec    { return ivec{x: a.x * f, y: a.y * f, z: a.z * f} }
func (a ivec) DivScalar(f int) ivec    { return ivec{x: a.x / f, y: a.y / f, z: a.z / f} }
func (a ivec) ShiftRight(lo int) ivec  { return ivec{x: a.x >> lo, y: a.y >> lo, z: a.z >> lo} }
func (a ivec) ShiftLeft(hi int) ivec   { return ivec{x: a.x << hi, y: a.y << hi, z: a.z << hi} }
func (a ivec) Sub(b ivec) ivec         { return ivec{x: a.x - b.x, y: a.y - b.y, z: a.z - b.z} }
func (a ivec) Vec() ms3.Vec            { return ms3.Vec{X: float32(a.x), Y: float32(a.y), Z: float32(a.z)} }
func (a ivec) AndScalar(f int) ivec    { return ivec{x: a.x & f, y: a.y & f, z: a.z & f} }
func (a ivec) OrScalar(f int) ivec     { return ivec{x: a.x | f, y: a.y | f, z: a.z | f} }
func (a ivec) XorScalar(f int) ivec    { return ivec{x: a.x ^ f, y: a.y ^ f, z: a.z ^ f} }
func (a ivec) AndnotScalar(f int) ivec { return ivec{x: a.x &^ f, y: a.y &^ f, z: a.z &^ f} }

type icube struct {
	ivec
	lvl int
}

func (c icube) isSmallest() bool       { return c.lvl == 1 }
func (c icube) isSecondSmallest() bool { return c.lvl == 2 }

// decomposesTo returns the amount of smallest level cubes generated from decomposing this cube completely.
func (c icube) decomposesTo(targetLvl int) int {
	if targetLvl > c.lvl {
		panic("invalid targetLvl to icube.decomposesTo")
	}
	return int(pow8(c.lvl - targetLvl))
}

func (c icube) size(baseRes float32) float32 {
	dim := 1 << (c.lvl - 1)
	return float32(dim) * baseRes
}

// supercube returns the icube's parent octree icube.
func (c icube) supercube() icube {
	upLvl := c.lvl + 1
	bitmask := (1 << upLvl) - 1
	return icube{
		ivec: c.ivec.AndnotScalar(bitmask),
		lvl:  upLvl,
	}
}

func (c icube) box(origin ms3.Vec, size float32) ms3.Box {
	origin = c.origin(origin, size) // Replace origin with icube origin.
	return ms3.Box{
		Min: origin,
		Max: ms3.AddScalar(size, origin),
	}
}

func (c icube) origin(origin ms3.Vec, size float32) ms3.Vec {
	idx := c.lvlIdx()
	return ms3.Add(origin, ms3.Scale(size, idx.Vec()))
}

func (c icube) lvlIdx() ivec {
	return c.ivec.ShiftRight(c.lvl) // icube indices per level in the octree.
}

func (c icube) center(origin ms3.Vec, size float32) ms3.Vec {
	return c.box(origin, size).Center() // TODO(soypat): this can probably be optimized.
}

// corners returns the cube corners. Be aware size is NOT the minimum cube resolution but
// can be calculated with the [icube.size] method using resolution. If [icube.lvl]==1 then size is resolution.
func (c icube) corners(origin ms3.Vec, size float32) [8]ms3.Vec {
	origin = c.origin(origin, size)
	return [8]ms3.Vec{
		ms3.Add(origin, ms3.Vec{X: 0, Y: 0, Z: 0}),
		ms3.Add(origin, ms3.Vec{X: size, Y: 0, Z: 0}),
		ms3.Add(origin, ms3.Vec{X: size, Y: size, Z: 0}),
		ms3.Add(origin, ms3.Vec{X: 0, Y: size, Z: 0}),
		ms3.Add(origin, ms3.Vec{X: 0, Y: 0, Z: size}),
		ms3.Add(origin, ms3.Vec{X: size, Y: 0, Z: size}),
		ms3.Add(origin, ms3.Vec{X: size, Y: size, Z: size}),
		ms3.Add(origin, ms3.Vec{X: 0, Y: size, Z: size}),
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
