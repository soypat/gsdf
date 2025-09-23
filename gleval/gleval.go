package gleval

import (
	"errors"
	"fmt"
	"slices"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
)

// SDF3 implements a 3D signed distance field in vectorized
// form suitable for running on GPU.
type SDF3 interface {
	// Evaluate evaluates the signed distance field over pos positions.
	// dist and pos must be of same length.  Resulting distances are stored
	// in dist.
	//
	// userData facilitates getting data to the evaluators for use in processing, such as [VecPool].
	Evaluate(pos []ms3.Vec, dist []float32, userData any) error
	// Bounds returns the SDF's bounding box such that all of the shape is contained within.
	Bounds() ms3.Box
}

// SDF2 implements a 2D signed distance field in vectorized
// form suitable for running on GPU.
type SDF2 interface {
	// Evaluate evaluates the signed distance field over pos positions.
	// dist and pos must be of same length.  Resulting distances are stored
	// in dist.
	//
	// userData facilitates getting data to the evaluators for use in processing, such as [VecPool].
	Evaluate(pos []ms2.Vec, dist []float32, userData any) error
	// Bounds returns the SDF's bounding box such that all of the shape is contained within.
	Bounds() ms2.Box
}

// These interfaces are implemented by all SDF interfaces such as SDF3/2 and Shader3D/2D.
// Using these instead of `any` Aids in catching mistakes at compile time such as passing a Shader3D instead of Shader2D as an argument.
type (
	bounder2 = interface{ Bounds() ms2.Box }
	bounder3 = interface{ Bounds() ms3.Box }
)

var (
	errEmptyBuffers         = errors.New("empty buffers")
	errMismatchBufferLength = errors.New("position and distance buffer length mismatch")
)

// NormalsCentralDiff uses central differences algorithm for normal calculation, which are stored in normals for each position.
// The returned normals are not normalized (converted to unit length).
func NormalsCentralDiff(s SDF3, pos []ms3.Vec, normals []ms3.Vec, step float32, userData any) error {
	step *= 0.5
	if step <= 0 {
		return errors.New("invalid step")
	} else if len(pos) != len(normals) {
		return errors.New("length of position must match length of normals")
	} else if s == nil {
		return errors.New("nil SDF3")
	} else if len(pos) == 0 {
		return errEmptyBuffers
	}
	vp, err := GetVecPool(userData)
	if err != nil {
		return fmt.Errorf("VecPool required in both GPU and CPU situations for Normal calculation: %s", err)
	}
	d1 := vp.Float.Acquire(len(pos))
	d2 := vp.Float.Acquire(len(pos))
	auxPos := vp.V3.Acquire(len(pos))
	defer vp.Float.Release(d1)
	defer vp.Float.Release(d2)
	defer vp.V3.Release(auxPos)
	var vecs = [3]ms3.Vec{{X: step}, {Y: step}, {Z: step}}
	for dim := 0; dim < 3; dim++ {
		h := vecs[dim]
		for i, p := range pos {
			auxPos[i] = ms3.Add(p, h)
		}
		err = s.Evaluate(auxPos, d1, userData)
		if err != nil {
			return err
		}
		for i, p := range pos {
			auxPos[i] = ms3.Sub(p, h)
		}
		err = s.Evaluate(auxPos, d2, userData)
		if err != nil {
			return err
		}

		switch dim {
		case 0:
			for i, d := range d1 {
				normals[i].X = d - d2[i]
			}
		case 1:
			for i, d := range d1 {
				normals[i].Y = d - d2[i]
			}
		case 2:
			for i, d := range d1 {
				normals[i].Z = d - d2[i]
			}
		}
	}
	return nil
}

type BlockCachedSDF3 struct {
	sdf     SDF3
	mul     ms3.Vec
	m       map[[3]int]float32
	posbuf  []ms3.Vec
	distbuf []float32
	idxbuf  []int
	hits    uint64
	evals   uint64
}

func (c3 *BlockCachedSDF3) VecPool() *VecPool {
	vp, _ := GetVecPool(c3.sdf)
	return vp
}

// Reset resets the SDF3 and reuses the underlying buffers for future SDF evaluations. It also resets statistics such as evaluations and cache hits.
func (c3 *BlockCachedSDF3) Reset(sdf SDF3, resX, resY, resZ float32) error {
	if resX <= 0 || resY <= 0 || resZ <= 0 {
		return errors.New("invalid resolution for BlockCachedSDF3")
	}
	if c3.m == nil {
		c3.m = make(map[[3]int]float32)
	} else {
		clear(c3.m)
	}
	// bb := sdf.Bounds()
	// Ncells := ms3.DivElem(bb.Size(), res)
	*c3 = BlockCachedSDF3{
		sdf:     sdf,
		mul:     ms3.DivElem(ms3.Vec{X: 1, Y: 1, Z: 1}, ms3.Vec{X: resX, Y: resY, Z: resZ}),
		m:       c3.m,
		posbuf:  c3.posbuf[:0],
		distbuf: c3.distbuf[:0],
		idxbuf:  c3.idxbuf[:0],
	}
	return nil
}

// CacheHits returns total amount of cached evalutions done throughout the SDF's lifetime.
func (c3 *BlockCachedSDF3) CacheHits() uint64 {
	return c3.hits
}

// Evaluations returns total evaluations performed succesfully during sdf's lifetime, including cached.
func (c3 *BlockCachedSDF3) Evaluations() uint64 {
	return c3.evals
}

// Evaluate implements the [SDF3] interface with cached evaluation.
func (c3 *BlockCachedSDF3) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	if len(pos) != len(dist) {
		return errMismatchBufferLength
	} else if len(pos) == 0 {
		return errEmptyBuffers
	}
	bb := c3.sdf.Bounds()
	seekPos := c3.posbuf[:0]
	idx := c3.idxbuf[:0]
	mul := c3.mul
	for i, p := range pos {
		tp := ms3.MulElem(mul, ms3.Sub(p, bb.Min))
		k := [3]int{
			int(tp.X),
			int(tp.Y),
			int(tp.Z),
		}
		d, cached := c3.m[k]
		if cached {
			dist[i] = d
		} else {
			seekPos = append(seekPos, p)
			idx = append(idx, i)
		}
	}
	if len(idx) > 0 {
		// Renew buffers in case they were grown.
		c3.idxbuf = idx
		c3.posbuf = seekPos
		c3.distbuf = slices.Grow(c3.distbuf[:0], len(seekPos))
		seekDist := c3.distbuf[:len(seekPos)]
		err := c3.sdf.Evaluate(seekPos, seekDist, userData)
		if err != nil {
			return err
		}
		// Add new entries to cache.
		for i, p := range seekPos {
			tp := ms3.MulElem(mul, ms3.Sub(p, bb.Min))
			k := [3]int{
				int(tp.X),
				int(tp.Y),
				int(tp.Z),
			}
			c3.m[k] = seekDist[i]
		}
		// Fill original buffer with new distances.
		for i, d := range seekDist {
			dist[idx[i]] = d
		}
	}
	c3.evals += uint64(len(dist))
	c3.hits += uint64(len(dist) - len(seekPos))
	return nil
}

// Bounds returns the SDF's bounding box such that all of the shape is contained within.
func (c3 *BlockCachedSDF3) Bounds() ms3.Box {
	return c3.sdf.Bounds()
}

type cachedExactSDF3 struct {
	SDF     SDF3
	m       map[[3]uint32]float32
	posbuf  []ms3.Vec
	distbuf []float32
	idxbuf  []int
	hits    uint64
	evals   uint64
}

// CacheHits returns total amount of cached evalutions done throughout the SDF's lifetime.
func (c3 *cachedExactSDF3) CacheHits() uint64 {
	return c3.hits
}

// Evaluations returns total evaluations performed succesfully during sdf's lifetime, including cached.
func (c3 *cachedExactSDF3) Evaluations() uint64 {
	return c3.evals
}

// Evaluate implements the [SDF3] interface with cached evaluation.
func (c3 *cachedExactSDF3) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	if len(pos) != len(dist) {
		return errMismatchBufferLength
	} else if len(pos) == 0 {
		return errEmptyBuffers
	}
	if c3.m == nil {
		c3.m = make(map[[3]uint32]float32)
	}
	seekPos := c3.posbuf[:0]
	idx := c3.idxbuf[:0]
	for i, p := range pos {
		k := [3]uint32{
			math32.Float32bits(p.X),
			math32.Float32bits(p.Y),
			math32.Float32bits(p.Z),
		}
		d, cached := c3.m[k]
		if cached {
			dist[i] = d
		} else {
			seekPos = append(seekPos, p)
			idx = append(idx, i)
		}
	}
	if len(idx) > 0 {
		// Renew buffers in case they were grown.
		c3.idxbuf = idx
		c3.posbuf = seekPos
		c3.distbuf = slices.Grow(c3.distbuf[:0], len(seekPos))
		seekDist := c3.distbuf[:len(seekPos)]
		err := c3.SDF.Evaluate(seekPos, seekDist, userData)
		if err != nil {
			return err
		}
		// Add new entries to cache.
		for i, p := range seekPos {
			k := [3]uint32{
				math32.Float32bits(p.X),
				math32.Float32bits(p.Y),
				math32.Float32bits(p.Z),
			}
			c3.m[k] = seekDist[i]
		}
		// Fill original buffer with new distances.
		for i, d := range seekDist {
			dist[idx[i]] = d
		}
	}
	c3.evals += uint64(len(dist))
	c3.hits += uint64(len(dist) - len(seekPos))
	return nil
}

// Bounds returns the SDF's bounding box such that all of the shape is contained within.
func (c3 *cachedExactSDF3) Bounds() ms3.Box {
	return c3.SDF.Bounds()
}
