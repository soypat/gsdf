package glrender

import (
	"errors"
	"io"
	"os"
	"slices"

	"github.com/soypat/geometry/ms3"
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
		oc.prunecubes = make([]icube, pruneSize)
	}

	// Each level contains 8 cubes.
	// In DFS descent we need only choose one cube per level with current algorithm.
	// Future algorithm may see this number grow to match evaluation buffers for cube culling.
	minCubesSize := levels * 8
	if cap(oc.cubes) < minCubesSize {
		oc.cubes = make([]icube, minCubesSize)
	}

	*oc = Octree{
		s:          s,
		origin:     origin,
		bounds:     bb,
		resolution: cubeResolution,
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
		oc.prunecubes, ok = octreeDecomposeBFS(oc.prunecubes, prunable, minPrunableLvl)
		if ok {
			oc.cubes[upi].lvl = 0 // Mark as used in prune buffer.
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
		oc.posbuf, oc.cubes = octreeDecomposeDFS(oc.posbuf, oc.cubes, oc.origin, oc.resolution)

		// Limit evaluation to what is needed by this call to ReadTriangles.
		currentLim := min(8*(len(dst)-n), aligndown(len(oc.posbuf), 8))
		if currentLim == 0 {
			panic("zero buffer")
		}
		err = oc.s.Evaluate(oc.posbuf[:currentLim], oc.distbuf[:currentLim], userData)
		if err != nil {
			return 0, err
		}
		nt, k := marchCubes(dst[n:], oc.posbuf[:currentLim], oc.distbuf[:currentLim], oc.resolution)
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
	unpruned, smallestPruned, err := octreePrune(oc.s, oc.prunecubes, oc.origin, oc.resolution, pos, oc.distbuf[:len(pos)], userData, szDistMult, false)
	oc.prunecubes = unpruned
	oc.pruned += smallestPruned
	return err
}

// refillUnpruned takes cubes that were left unpruned and fills empty spots in cubes buffer with them.
func (oc *Octree) refillCubesWithUnpruned() {
	if len(oc.prunecubes) == 0 {
		return
	}
	oc.cubes, oc.prunecubes, oc.markedToPrune = octreeSafeSpread(oc.cubes, oc.prunecubes, oc.markedToPrune)
	if len(oc.cubes) == 0 {
		oc.cubes, oc.prunecubes = octreeSafeMove(oc.cubes, oc.prunecubes) // TODO(soypat): fix bug that causes safe move to not be so safe, overflows cubes in decomp.
	}
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

func (oc *Octree) done() bool {
	return len(oc.cubes) == 0 && len(oc.posbuf) == 0 && len(oc.prunecubes) == 0
}

// DebugVisual not guaranteed to stay.
func (oc *Octree) debugVisual(filename string, lvlDescent int, merge glbuild.Shader3D, bld *gsdf.Builder) error {
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
	bb := bld.NewBoundsBoxFrame(topBB)
	s := bld.NewSphere(res / 2)
	s = bld.Translate(s, origin.X, origin.Y, origin.Z)
	s = bld.Union(s, bb)
	if merge != nil {
		s = bld.Union(s, merge)
	}
	for _, c := range cubes {
		bb := bld.NewBoundsBoxFrame(c.box(origin, c.size(res)))
		s = bld.Union(s, bb)
	}
	s = bld.Scale(s, 0.5/s.Bounds().Size().Max())
	glbuild.ShortenNames3D(&s, 12)
	prog := glbuild.NewDefaultProgrammer()
	fp, err := os.Create(filename)
	if err != nil {
		return err
	}
	_, ssbos, err := prog.WriteShaderToyVisualizerSDF3(fp, s)
	if err != nil {
		return err
	} else if slices.ContainsFunc(ssbos, func(b glbuild.ShaderObject) bool { return b.IsBindable() }) {
		return errors.New("bindable object unsupported for visual output")
	}
	return nil
}

var _pow8 = [...]uint64{
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
