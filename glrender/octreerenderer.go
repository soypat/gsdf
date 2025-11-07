package glrender

import (
	"errors"
	"io"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/i3"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/gleval"
)

const minPrunableLvl = 3

// Octree is a marching-triangles Octree implementation with sub-cube pruning.
type Octree struct {
	s   gleval.SDF3
	oct ms3.Octree

	bounds ms3.Box
	// cubes stores cubes decomposed in a depth first search(DFS). It's length is chosen such that
	// decomposing an octree branch in DFS down to the smallest octree unit will use up the entire buffer.
	cubes []i3.Cube
	// levels is the octree total amount of cube levels.

	// markedToPrune is a counter that keeps track of total amount of cubes in cubes buffer that
	// have been marked as moved to prunecubes buffer for pruning.
	markedToPrune int
	// prunecubes stores icubes to be pruned via a breadth first search in an independent buffer.
	// During a call to prune they map directly to position/distance buffer.
	prunecubes []i3.Cube

	// Below are the buffers for storing positional input to SDF and resulting distances.

	// posbuf's length accumulates positions to be evaluated. May me non-zero length between ReadTriangles calls
	// which implies there are positions to be evaluated and marched.
	posbuf []ms3.Vec
	// distbuf is set to the calculated distances for posbuf. length==capacity always.
	distbuf []float32
	// pruned statistics: quantity of minimum resolution cubes pruned from octree and their calculations omitted (x8).
	pruned uint64
}

// NewOctreeRenderer instantiates a new Octree renderer for rendering triangles from an [gleval.SDF3].
func NewOctreeRenderer(s gleval.SDF3, cubeResolution float32, evalBufferSize int) (*Octree, error) {
	if evalBufferSize < 64 {
		return nil, errors.New("bad octree eval buffer size")
	}

	_, _, err := makeICube(s.Bounds(), cubeResolution) // Early error check before allocating eval buffers.
	if err != nil {
		return nil, err
	}
	var oc Octree
	oc.posbuf = make([]ms3.Vec, evalBufferSize)[:0]
	oc.distbuf = make([]float32, cap(oc.posbuf))
	err = oc.Reset(s, cubeResolution)
	if err != nil {
		return nil, err
	}
	return &oc, nil
}

// TotalPruned returns the amount of minimum resolution cubes pruned throughout the rendering of the current SDF3.
// This number is reset on a call to Reset.
func (oc *Octree) TotalPruned() uint64 {
	return oc.pruned
}

// Reset switched the underlying SDF3 for a new one with a new cube resolution. It reuses
// the same evaluation buffers and cube buffer if it can.
func (oc *Octree) Reset(s gleval.SDF3, cubeResolution float32) error {
	if cubeResolution <= 0 {
		return errors.New("invalid renderer cube resolution")
	}
	// Scale the bounding box about the center to make sure the boundaries
	// aren't on the object surface.

	bb := s.Bounds()
	bb = bb.ScaleCentered(ms3.Vec{X: 1.01, Y: 1.01, Z: 1.01})
	// halfResVec := ms3.Vec{X: 0.5 * cubeResolution, Y: 0.5 * cubeResolution, Z: 0.5 * cubeResolution}
	// bb.Max = ms3.Add(bb.Max, halfResVec)
	// bb.Min = ms3.Sub(bb.Min, halfResVec)

	topCube, origin, err := makeICube(bb, cubeResolution)
	if err != nil {
		return err
	}
	levels := topCube.Level

	// We only try pruning cubes of at min level 3, so a top level cube of size 4
	// will be the minimium level to enable pruning. Due to memory constraints
	// we can prune completely a level, at most.
	var tblPruneSize = [...]int{
		minPrunableLvl + 1: 8,
		minPrunableLvl + 2: 8 + 8*8,                   // 8+64=72
		minPrunableLvl + 3: 8 + 8*8 + 8*8*8,           // 72+512=584
		minPrunableLvl + 4: 8 + 8*8 + 8*8*8 + 8*8*8*8, // 584+4096=4680
		// 8: 8 + 8*8 + 8*8*8 + 8*8*8*8 + 8*8*8*8*8, // 4680+32768=37448
	}
	pruneSize := tblPruneSize[min(levels, len(tblPruneSize)-1)]
	pruneSize = min(pruneSize, aligndown(len(oc.distbuf), 8)) // Can't prune more cubes than distance buffer allows.
	if cap(oc.prunecubes) < pruneSize {
		oc.prunecubes = make([]i3.Cube, pruneSize)
	}

	// Each level contains 8 cubes.
	// In DFS descent we need only choose one cube per level with current algorithm.
	// Future algorithm may see this number grow to match evaluation buffers for cube culling.
	minCubesSize := levels * 8
	if cap(oc.cubes) < minCubesSize {
		oc.cubes = make([]i3.Cube, minCubesSize)
	}

	*oc = Octree{
		s:          s,
		oct:        ms3.Octree{Resolution: cubeResolution, Origin: origin},
		bounds:     bb,
		cubes:      oc.cubes[:1],
		prunecubes: oc.prunecubes[:0],

		// Reuse distbuf and posbuf.
		distbuf: oc.distbuf,
		posbuf:  oc.posbuf[:0],
	}

	oc.cubes[0] = topCube // Start cube.
	return nil
}

func (oc *Octree) ReadTriangles(dst []ms3.Triangle, userData any) (n int, err error) {
	if len(dst) < 5 {
		return 0, io.ErrShortBuffer
	}

	upi := oc.nextUnpruned()
	if upi >= 0 && len(oc.prunecubes) == 0 {
		prunable := oc.cubes[upi]
		var ok bool
		oc.prunecubes, ok = oc.oct.DecomposeBFS(oc.prunecubes, prunable, minPrunableLvl)
		if ok {
			oc.cubes[upi].Level = 0 // Mark as used in prune buffer.
			oc.markedToPrune++
		}
	}
	if len(oc.prunecubes) > 0 {
		err = oc.prune(userData)
		if err != nil {
			return 0, err
		}
		oc.refillCubesWithUnpruned()
	}

	for len(dst)-n > 5 {
		if oc.done() {
			return n, io.EOF // Done rendering model.
		}
		if len(oc.cubes) == 0 {
			oc.refillCubesWithUnpruned()
		}
		oc.posbuf, oc.cubes = oc.oct.DecomposeDFS(oc.posbuf, oc.cubes)

		// Limit evaluation to what is needed by this call to ReadTriangles.
		currentLim := min(8*(len(dst)-n), aligndown(len(oc.posbuf), 8))
		if currentLim == 0 {
			panic("zero buffer")
		}
		err = oc.s.Evaluate(oc.posbuf[:currentLim], oc.distbuf[:currentLim], userData)
		if err != nil {
			return 0, err
		}
		nt, k := marchCubes(dst[n:], oc.posbuf[:currentLim], oc.distbuf[:currentLim], oc.oct.Resolution)
		n += nt
		k = copy(oc.posbuf, oc.posbuf[k:])
		oc.posbuf = oc.posbuf[:k]
	}
	return n, nil
}

func (oc *Octree) prune(userData any) (err error) {
	// The cubes pruned must not contain a surface within.
	const szDistMult = sqrt3 / 2
	pos := oc.posbuf[len(oc.posbuf):cap(oc.posbuf)] // Use free space in position buffer.
	if len(pos) < len(oc.prunecubes) {
		return nil
	}
	unpruned, smallestPruned, err := octreePrunea(oc.s, oc.prunecubes, oc.oct.Origin, oc.oct.Resolution, pos, oc.distbuf[:len(pos)], userData, szDistMult, false)
	oc.prunecubes = unpruned
	oc.pruned += smallestPruned
	return err
}

// refillUnpruned takes cubes that were left unpruned and fills empty spots in cubes buffer with them.
func (oc *Octree) refillCubesWithUnpruned() {
	if len(oc.prunecubes) == 0 {
		return
	}
	oc.cubes, oc.prunecubes, oc.markedToPrune = oc.oct.SafeSpread(oc.cubes, oc.prunecubes, oc.markedToPrune)
	if len(oc.cubes) == 0 {
		oc.cubes, oc.prunecubes = oc.oct.SafeMove(oc.cubes, oc.prunecubes)
	}
}

func (oc *Octree) nextUnpruned() int {
	if cap(oc.prunecubes) == 0 {
		return -1 // Pruning disabled.
	}
	for i := 0; i < len(oc.cubes); i++ {
		if oc.cubes[i].Level >= minPrunableLvl {
			return i // We might be able to prune cube.
		}
	}
	return -1
}

func (oc *Octree) done() bool {
	return len(oc.cubes) == 0 && len(oc.posbuf) == 0 && len(oc.prunecubes) == 0
}

// This file contains basic low level algorithms regarding Octrees.

func makeICube(bb ms3.Box, minResolution float32) (topCube i3.Cube, origin ms3.Vec, err error) {
	if minResolution <= 0 || math32.IsNaN(minResolution) || math32.IsInf(minResolution, 0) {
		return i3.Cube{}, ms3.Vec{}, errors.New("invalid renderer cube resolution")
	}
	sz := bb.Size()
	longAxis := sz.Max()
	// how many cube levels for the octree?
	log2 := math32.Log2(longAxis / minResolution)
	levels := int(math32.Ceil(log2)) + 1
	if levels <= 1 {
		return i3.Cube{}, ms3.Vec{}, errors.New("resolution not fine enough for marching cubes")
	}
	return i3.Cube{Level: levels}, bb.Min, nil
}

// octreePrune discards cubes in prune that contain no surface within a distance of CubeDimension * szMultMaxDist of the cube center.
// It returns the modified prune buffer containing unpruned cubes and the calculated number of smallest-level cubes pruned in the process.
// If useOriginInsteadOfCenter is set to true the distance comparison is done against the voxel/cube origin instead of center.
func octreePrunea(s gleval.SDF3, toPrune []i3.Cube, origin ms3.Vec, res float32, posBuf []ms3.Vec, distbuf []float32, userData any, szMultMaxDist float32, useOriginInsteadOfCenter bool) (unpruned []i3.Cube, smallestPruned uint64, err error) {
	if len(toPrune) == 0 {
		return toPrune, 0, nil
	} else if len(posBuf) < len(toPrune) {
		return toPrune, 0, errors.New("positional buffer length must be greater than prune cubes length")
	} else if len(posBuf) != len(distbuf) {
		return toPrune, 0, errors.New("positional buffer must match distance buffer length")
	}
	oct := ms3.Octree{
		Resolution: res,
		Origin:     origin,
	}
	posBuf = posBuf[:len(toPrune)]
	distbuf = distbuf[:len(toPrune)]
	if useOriginInsteadOfCenter {
		for i, p := range toPrune {
			posBuf[i] = oct.CubeOrigin(p, oct.CubeSize(p))
		}
	} else {
		for i, p := range toPrune {
			posBuf[i] = oct.CubeCenter(p, oct.CubeSize(p))
		}
	}

	err = s.Evaluate(posBuf, distbuf, userData)
	if err != nil {
		return toPrune, 0, err
	}
	// Move filled cubes to front and prune empty cubes.
	runningIdx := 0
	for i, p := range toPrune {
		size := oct.CubeSize(p)
		maxDist := size * szMultMaxDist
		isPrunable := math32.Abs(distbuf[i]) >= maxDist
		if !isPrunable {
			// Cube not empty, do not discard.
			toPrune[runningIdx] = p
			runningIdx++
		} else {
			smallestPruned += p.DecomposesTo(1)
		}
	}
	toPrune = toPrune[:runningIdx]
	return toPrune, smallestPruned, nil
}
