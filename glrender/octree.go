package glrender

import (
	"errors"
	"io"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/gleval"
)

type Octree struct {
	s          gleval.SDF3
	origin     ms3.Vec
	levels     int
	resolution float32
	cubes      []icube
	// Below are the buffers for storing positional input to SDF and resulting distances.

	// posbuf's length accumulates positions to be evaluated.
	posbuf []ms3.Vec
	// distbuf is set to the calculated distances for posbuf.
	distbuf []float32
}

// NewOctreeRenderer instantiates a new Octree renderer for rendering triangles from an [gleval.SDF3].
func NewOctreeRenderer(s gleval.SDF3, cubeResolution float32, evalBufferSize int) (*Octree, error) {
	if evalBufferSize < 64 {
		return nil, errors.New("bad octree eval buffer size")
	} else if cubeResolution <= 0 {
		return nil, errors.New("invalid renderer cube resolution")
	}
	var oc Octree
	err := oc.Reset(s, cubeResolution)
	if err != nil {
		return nil, err
	}
	oc.posbuf = make([]ms3.Vec, 0, evalBufferSize)
	oc.distbuf = make([]float32, evalBufferSize)
	return &oc, nil
}

// Reset switched the underlying SDF3 for a new one with a new cube resolution. It reuses
// the same evaluation buffers and cube buffer if it can.
func (oc *Octree) Reset(s gleval.SDF3, cubeResolution float32) error {
	if cubeResolution <= 0 {
		return errors.New("invalid renderer cube resolution")
	}
	// Scale the bounding box about the center to make sure the boundaries
	// aren't on the object surface.
	bb := s.Bounds().ScaleCentered(ms3.Vec{X: 1.01, Y: 1.01, Z: 1.01})
	longAxis := bb.Size().Max()

	// how many cube levels for the octree?
	log2 := math32.Log2(longAxis / cubeResolution)
	levels := int(math32.Ceil(log2))
	if levels <= 1 {
		return errors.New("resolution not fine enough for marching cubes")
	}

	// Each level contains 8 cubes.
	// In DFS descent we need only choose one cube per level with current algorithm.
	// Future algorithm may see this number grow to match evaluation buffers for cube culling.
	minCubesSize := levels * 8
	if cap(oc.cubes) < minCubesSize {
		oc.cubes = make([]icube, 0, minCubesSize)
	}
	oc.cubes = oc.cubes[:1]
	oc.cubes[0] = icube{lvl: levels} // Start cube.
	oc.s = s
	oc.resolution = cubeResolution
	oc.levels = levels
	oc.origin = bb.Min
	return nil
}

func (oc *Octree) ReadTriangles(dst []ms3.Triangle) (n int, err error) {
	if len(dst) < 5 {
		return 0, io.ErrShortBuffer
	}
	for len(dst)-n > 5 {
		if oc.done() {
			return n, io.EOF // Done rendering model.
		}
		oc.processCubesDFS()
		// Limit evaluation to what is needed by this call to ReadTriangles.
		posLimit := min(8*(len(dst)-n), aligndown(len(oc.posbuf), 8))
		err = oc.s.Evaluate(oc.posbuf[:posLimit], oc.distbuf[:posLimit], nil)
		if err != nil {
			return 0, err
		}
		n += oc.marchCubes(dst[n:], posLimit)
	}
	return n, nil
}

// processCubesDFS decomposes cubes in the buffer into more cubes. Base-level cubes
// are decomposed into corners in position buffer for marching cubes algorithm. It uses Depth First Search.
func (oc *Octree) processCubesDFS() {
	origin, res := oc.origin, oc.resolution
	for len(oc.cubes) > 0 {
		lastIdx := len(oc.cubes) - 1
		cube := oc.cubes[lastIdx]
		subCubes := cube.octree()
		if subCubes[0].lvl == 1 {
			// Is base-level cube.
			if cap(oc.posbuf)-len(oc.posbuf) < 8*8 {
				break // No space for position buffering.
			}
			for _, scube := range subCubes {
				corners := scube.corners(origin, res)
				oc.posbuf = append(oc.posbuf, corners[:]...)
			}
			oc.cubes = oc.cubes[:lastIdx] // Trim cube used.
		} else {
			// Is cube with sub-cubes.
			if cap(oc.cubes)-len(oc.cubes) < 8 {
				break // No more space for cube buffering.
			}
			// We trim off the last cube which we just processed in append.
			oc.cubes = append(oc.cubes[:lastIdx], subCubes[:]...)
		}
	}
}

func (oc *Octree) marchCubes(dst []ms3.Triangle, limit int) int {
	nTri := 0
	var p [8]ms3.Vec
	var d [8]float32
	cubeDiag := 2 * sqrt3 * oc.resolution
	iPos := 0
	for iPos < limit && len(dst)-nTri > marchingCubesMaxTriangles {
		if math32.Abs(oc.distbuf[iPos]) <= cubeDiag {
			// Cube may have triangles.
			copy(p[:], oc.posbuf[iPos:iPos+8])
			copy(d[:], oc.distbuf[iPos:iPos+8])
			nTri += mcToTriangles(dst[nTri:], p, d, 0)
		}
		iPos += 8
	}
	remaining := len(oc.posbuf) - iPos
	if remaining > 0 {
		// Discard used positional and distance data.
		k := copy(oc.posbuf, oc.posbuf[iPos:])
		oc.posbuf = oc.posbuf[:k]
	} else {
		oc.posbuf = oc.posbuf[:0] // Reset buffer.
	}
	return nTri
}

func (oc *Octree) done() bool {
	return len(oc.cubes) == 0 && len(oc.posbuf) == 0
}
