package glrender

import (
	"errors"
	"io"
	"os"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
)

const minPrunableLvl = 3

// Octree is a marching-triangles Octree implementation with sub-cube pruning.
type Octree struct {
	s          gleval.SDF3
	origin     ms3.Vec
	bounds     ms3.Box
	resolution float32
	// cubes stores cubes decomposed in a depth first search(DFS). It's length is chosen such that
	// decomposing an octree branch in DFS down to the smallest octree unit will use up the entire buffer.
	cubes []icube
	// levels is the octree total amount of cube levels.
	levels int
	// markedToPrune is a counter that keeps track of total amount of cubes in cubes buffer that
	// have been marked as moved to prunecubes buffer for pruning.
	markedToPrune int
	// prunecubes stores icubes to be pruned via a breadth first search in an independent buffer.
	// During a call to prune they map directly to position/distance buffer.
	prunecubes []icube

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
	oc.posbuf = make([]ms3.Vec, 0, evalBufferSize)
	oc.distbuf = make([]float32, evalBufferSize)
	err = oc.Reset(s, cubeResolution)
	if err != nil {
		return nil, err
	}
	return &oc, nil
}

func makeICube(bb ms3.Box, minResolution float32) (icube, ms3.Vec, error) {
	if minResolution <= 0 || math32.IsNaN(minResolution) || math32.IsInf(minResolution, 0) {
		return icube{}, ms3.Vec{}, errors.New("invalid renderer cube resolution")
	}
	longAxis := bb.Size().Max()
	// how many cube levels for the octree?
	log2 := math32.Log2(longAxis / minResolution)
	levels := int(math32.Ceil(log2)) + 1
	if levels <= 1 {
		return icube{}, ms3.Vec{}, errors.New("resolution not fine enough for marching cubes")
	}
	return icube{lvl: levels}, bb.Min, nil
}

// TotalPruned returns the amount of minimum resolution cubes pruned throughout the rendering of the current SDF3.
// This number is reset on a call to Reset. The amount of SDF evaluations omitted is roughly equivalent to 8*TotalPruned.
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
	topCube, origin, err := makeICube(bb, cubeResolution)
	if err != nil {
		return err
	}
	levels := topCube.lvl

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
		oc.prunecubes = make([]icube, 0, pruneSize)
	}

	// Each level contains 8 cubes.
	// In DFS descent we need only choose one cube per level with current algorithm.
	// Future algorithm may see this number grow to match evaluation buffers for cube culling.
	minCubesSize := levels * 8
	if cap(oc.cubes) < minCubesSize {
		oc.cubes = make([]icube, 0, minCubesSize)
	}

	*oc = Octree{
		s:          s,
		origin:     origin,
		bounds:     bb,
		resolution: cubeResolution,
		cubes:      oc.cubes[:1],
		prunecubes: oc.prunecubes[:0],
		levels:     levels,
		// Reuse distbuf and posbuf.
		distbuf: oc.distbuf,
		posbuf:  oc.posbuf[:0],
	}

	oc.cubes[0] = icube{lvl: levels} // Start cube.
	return nil
}

func (oc *Octree) ReadTriangles(dst []ms3.Triangle) (n int, err error) {
	if len(dst) < 5 {
		return 0, io.ErrShortBuffer
	}

	upi := oc.nextUnpruned()
	if upi >= 0 && len(oc.prunecubes) == 0 {
		var ok bool
		prunable := oc.cubes[upi]
		oc.prunecubes, ok = fillCubesBFS(oc.prunecubes, prunable)
		if ok {
			oc.cubes[upi].lvl = 0 // Mark as used in prune buffer.
			oc.markedToPrune++
		}
	}
	if len(oc.prunecubes) > 0 {
		err = oc.prune()
		if err != nil {
			return 0, err
		}
		oc.refillCubesWithUnpruned()
	}

	for len(dst)-n > 5 {
		if oc.done() {
			return n, io.EOF // Done rendering model.
		}
		if len(oc.cubes) > 0 {
			oc.processCubesDFS()
		}
		if len(oc.cubes) == 0 {
			oc.refillCubesWithUnpruned()
		}

		// Limit evaluation to what is needed by this call to ReadTriangles.
		posLimit := min(8*(len(dst)-n), aligndown(len(oc.posbuf), 8))
		if posLimit == 0 {
			panic("zero buffer")
		}
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
		if cube.lvl == 0 {
			// Cube has been moved to prune queue. Discard and keep going.
			oc.cubes = oc.cubes[:lastIdx]
			continue
		}
		subCubes := cube.octree()
		if subCubes[0].isSmallest() {
			// Is base-level cube.
			if cap(oc.posbuf)-len(oc.posbuf) < 8*8 {
				break // No space for position buffering.
			}
			for _, scube := range subCubes {
				corners := scube.corners(origin, res)
				oc.posbuf = append(oc.posbuf, corners[:]...)
			}
			oc.cubes = oc.cubes[:lastIdx] // Trim cube used.
			if len(oc.cubes) == 0 {
				oc.refillCubesWithUnpruned()
			}
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

func (oc *Octree) prune() error {
	prune := oc.prunecubes
	if len(prune) == 0 {
		return nil
	}
	origin, res := oc.origin, oc.resolution
	// Take care to not step on positions that still need to be marched.
	pos := oc.posbuf[len(oc.posbuf):cap(oc.posbuf)]
	if len(pos) < len(prune) {
		return nil // Not enough space to prune.
	}
	pos = pos[:len(prune)]
	for i, p := range prune {
		size := p.size(res)
		center := p.center(origin, size)
		pos[i] = center
	}
	err := oc.s.Evaluate(pos, oc.distbuf[:len(pos)], nil)
	if err != nil {
		return err
	}
	// Move filled cubes to front and prune empty cubes.
	runningIdx := 0
	for i, p := range prune {
		halfDiagonal := p.size(res) * (sqrt3 / 2)
		isEmpty := math32.Abs(oc.distbuf[i]) >= halfDiagonal
		if !isEmpty {
			// Cube not empty, do not discard.
			prune[runningIdx] = p
			runningIdx++
		} else {
			oc.pruned += pow8(p.lvl - 1)
		}
	}
	oc.prunecubes = prune[:runningIdx]
	return nil
}

// refillUnpruned takes cubes that were left unpruned and fills empty spots in cubes buffer with them.
func (oc *Octree) refillCubesWithUnpruned() {
	if len(oc.prunecubes) == 0 {
		return
	} else if len(oc.cubes) == 0 {
		// Calculate amount of cubes that would be generated in DFS
		// and check how many cubes may be added without overflowing buffer.
		pruneLvl := oc.prunecubes[0].lvl
		genCubes := 1 + pruneLvl*8 // plus one for appended cube.
		free := cap(oc.cubes) - genCubes
		trimIdx := max(0, len(oc.prunecubes)-free)
		oc.cubes = append(oc.cubes, oc.prunecubes[trimIdx:]...)
		oc.prunecubes = oc.prunecubes[:trimIdx]
		return
	}

	i := 0
	prune := oc.prunecubes
	prevLvl := oc.levels
	nextUnpruned := prune[len(prune)-1]
	for oc.markedToPrune > 0 && len(prune) > 0 && i < len(oc.cubes) {
		if oc.cubes[i].lvl == 0 && nextUnpruned.lvl < prevLvl {
			oc.cubes[i] = nextUnpruned
			prune = prune[:len(prune)-1]
			nextUnpruned = prune[len(prune)-1]
			oc.markedToPrune--
		} else if oc.cubes[i].lvl > 1 {
			prevLvl = oc.cubes[i].lvl
		}
		i++
	}
	oc.prunecubes = prune
}

func (oc *Octree) nextUnpruned() int {
	if cap(oc.prunecubes) == 0 {
		return -1 // Pruning disabled.
	}
	for i := 0; i < len(oc.cubes); i++ {
		if oc.cubes[i].lvl >= minPrunableLvl {
			return i // We might be able to prune cube.
		}
	}
	return -1
}

// fillCubesBFS decomposes start into octree cubes and appends them to dst.
func fillCubesBFS(dst []icube, start icube) ([]icube, bool) {
	if cap(dst) < 8 {
		return dst, false
	} else if start.lvl < minPrunableLvl {
		return dst, false // Cube already fully decomposed.
	}

	subCubes := start.octree()
	startIdx := len(dst)
	firstIdx := len(dst)
	dst = append(dst, subCubes[:]...) // Cubes will be of at minimum minPrunableLvl-1
	for cap(dst)-len(dst) >= 8 {
		// Decompose and append cubes.
		cube := dst[firstIdx]
		if cube.lvl <= minPrunableLvl {
			// Reached cube of minimum prunable level.
			break
		}
		subCubes := cube.octree()
		// Is cube with sub-cubes.
		// We trim off the last cube which we just processed in append.
		dst = append(dst, subCubes[:]...)
		firstIdx++
	}
	// Move cubes to start of buffer from where we started consuming them.
	n := copy(dst[startIdx:], dst[firstIdx:])
	dst = dst[:startIdx+n]
	return dst, true
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
	return len(oc.cubes) == 0 && len(oc.posbuf) == 0 && len(oc.prunecubes) == 0
}

// DebugVisual not guaranteed to stay.
func (oc *Octree) debugVisual(filename string, lvlDescent int, merge glbuild.Shader3D) error {
	if lvlDescent > 3 {
		return errors.New("too large level descent")
	}
	origin, res := oc.origin, oc.resolution
	startCube, _, err := makeICube(oc.bounds, res)
	if err != nil {
		return err
	}
	targetLevel := startCube.lvl - lvlDescent
	if targetLevel < 1 {
		targetLevel = 1
	}
	// func levelsVisual(filename string, startCube icube, targetLvl int, origin ms3.Vec, res float32) {
	topBB := startCube.box(origin, startCube.size(res))
	cubes := []icube{startCube}
	i := 0
	for cubes[i].lvl > targetLevel {
		subcubes := cubes[i].octree()
		cubes = append(cubes, subcubes[:]...)
		i++
	}
	cubes = cubes[i:]
	bb, _ := gsdf.NewBoundsBoxFrame(topBB)
	s, _ := gsdf.NewSphere(res / 2)
	s = gsdf.Translate(s, origin.X, origin.Y, origin.Z)
	s = gsdf.Union(s, bb)
	if merge != nil {
		s = gsdf.Union(s, merge)
	}
	for _, c := range cubes {
		bb, err := gsdf.NewBoundsBoxFrame(c.box(origin, c.size(res)))
		if err != nil {
			return err
		}
		s = gsdf.Union(s, bb)
	}
	s = gsdf.Scale(s, 0.5/s.Bounds().Size().Max())
	glbuild.ShortenNames3D(&s, 8)
	prog := glbuild.NewDefaultProgrammer()
	fp, err := os.Create(filename)
	if err != nil {
		return err
	}
	_, err = prog.WriteFragVisualizerSDF3(fp, s)
	if err != nil {
		return err
	}
	return nil
}

var _pow8 = []uint64{
	0:  1,
	1:  8,
	2:  8 * 8,
	3:  8 * 8 * 8,
	4:  8 * 8 * 8 * 8,
	5:  8 * 8 * 8 * 8 * 8,
	6:  8 * 8 * 8 * 8 * 8 * 8,
	7:  8 * 8 * 8 * 8 * 8 * 8 * 8,
	8:  8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	9:  8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	10: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	11: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	12: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	13: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	14: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	15: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	16: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	17: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	18: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	19: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	20: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	21: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8,
	// 22: 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8 * 8, // overflows
}

// pow8 returns 8**y.
func pow8(y int) uint64 {
	if y < len(_pow8) {
		return _pow8[y]
	}
	panic("overflow pow8")
}
