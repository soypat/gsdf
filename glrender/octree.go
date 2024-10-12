package glrender

import (
	"errors"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/gleval"
)

// This file contains basic low level algorithms regarding Octrees.

func makeICube(bb ms3.Box, minResolution float32) (topCube icube, origin ms3.Vec, err error) {
	if minResolution <= 0 || math32.IsNaN(minResolution) || math32.IsInf(minResolution, 0) {
		return icube{}, ms3.Vec{}, errors.New("invalid renderer cube resolution")
	}
	sz := bb.Size()
	longAxis := sz.Max()
	// how many cube levels for the octree?
	log2 := math32.Log2(longAxis / minResolution)
	levels := int(math32.Ceil(log2)) + 1
	if levels <= 1 {
		return icube{}, ms3.Vec{}, errors.New("resolution not fine enough for marching cubes")
	}
	return icube{lvl: levels}, bb.Min, nil
}

// octreeDecomposeDFS decomposes icubes from the end of cubes into their octree sub-icubes
// and appends them to the cubes buffer, resulting in a depth-first traversal (DFS) of the octree.
// This way cubes will contain the largest cubes at the start and the smallest cubes at the end (highest index).
// Cubes that reach the smallest size will be consumed and their 3D corners appended to dst. Smallest size cubes do not decompose into more icubes.
// cubes with level of zero are discarded and no action is taken.
//
// The icube decomposition continues until one or more of the following conditions are met:
//   - Smallest cube size is reached and the capacity in 3D dst can't store a resolution sized icube corners, calculated as cap(dst)-len(dst) < 64.
//   - Need to decompose a icube to more icubes but capacity of cubes buffer not enough to store an octree decomposition, calculated as cap(cubes)-len(cubes) < 8.
//   - cubes buffer has been fully consumed and is empty, calculated as len(cubes) == 0.
//
// This algorithm is HEAPLESS: this means dst and cubes buffer capacities are not modified.
func octreeDecomposeDFS(dst []ms3.Vec, cubes []icube, origin ms3.Vec, res float32) ([]ms3.Vec, []icube) {
	for len(cubes) > 0 {
		lastIdx := len(cubes) - 1
		cube := cubes[lastIdx]
		if cube.lvl == 0 {
			// Cube has been moved to prune queue. Discard and keep going.
			cubes = cubes[:lastIdx]
			continue
		}
		if cube.isSecondSmallest() {
			// Is base-level cube.
			if cap(dst)-len(dst) < 8*8 {
				break // No space for position buffering.
			}
			subcubes := cube.octree()
			for _, scube := range subcubes {
				corners := scube.corners(origin, res)
				dst = append(dst, corners[:]...)
			}
			cubes = cubes[:lastIdx] // Trim cube used.

		} else {
			// Is cube with sub-cubes.
			if cap(cubes)-len(cubes) < 8 {
				break // No more space for cube buffering.
			}
			subcubes := cube.octree()
			// We trim off the last cube which we just processed in append.
			cubes = append(cubes[:lastIdx], subcubes[:]...)
		}
	}
	return dst, cubes
}

// octreeDecomposeBFS decomposes start into octree cubes and appends them to dst without surpassing dst's slice capacity.
// Smallest cubes will remain at the highest index of dst. The boolean value returned indicates whether the
// argument start icube was able to be decomposed and its children added to dst.
func octreeDecomposeBFS(dst []icube, start icube, minimumDecomposedLvl int) ([]icube, bool) {
	if minimumDecomposedLvl < 1 {
		panic("bad minimumDecomposedLvl")
	}
	if cap(dst) < 8 {
		return dst, false // No space to decompose new cubes.
	} else if start.lvl <= minimumDecomposedLvl {
		return dst, false // Cube already fully decomposed.
	}

	subCubes := start.octree()
	startIdx := len(dst)
	firstIdx := len(dst)
	dst = append(dst, subCubes[:]...) // Cubes will be of at minimum minLvl-1
	for cap(dst)-len(dst) >= 8 {
		// Decompose and append cubes.
		cube := dst[firstIdx]
		if cube.lvl <= minimumDecomposedLvl {
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

// octreePruneNoSurface1 discards cubes in prune that contain no surface within its bounds by evaluating SDF once in the cube center.
// It returns the modified prune buffer and the calculated number of smallest-level cubes pruned in the process.
func octreePruneNoSurface1(s gleval.SDF3, toPrune []icube, origin ms3.Vec, res float32, pos []ms3.Vec, distbuf []float32) (unpruned []icube, smallestPruned uint64, err error) {
	if len(toPrune) == 0 {
		return toPrune, 0, nil
	} else if len(pos) < len(toPrune) {
		return toPrune, 0, nil // errors.New("positional buffer length must be greater than prune cubes length")
	} else if len(pos) != len(distbuf) {
		return toPrune, 0, errors.New("positional buffer must match distance buffer length")
	}
	pos = pos[:len(toPrune)]
	distbuf = distbuf[:len(toPrune)]
	for i, p := range toPrune {
		size := p.size(res)
		center := p.center(origin, size)
		pos[i] = center
	}
	err = s.Evaluate(pos, distbuf, nil)
	if err != nil {
		return toPrune, 0, err
	}
	// Move filled cubes to front and prune empty cubes.
	runningIdx := 0
	for i, p := range toPrune {
		halfDiagonal := p.size(res) * (sqrt3 / 2)
		isEmpty := math32.Abs(distbuf[i]) >= halfDiagonal
		if !isEmpty {
			// Cube not empty, do not discard.
			toPrune[runningIdx] = p
			runningIdx++
		} else {
			smallestPruned += pow8(p.lvl - 1)
		}
	}
	toPrune = toPrune[:runningIdx]
	return toPrune, smallestPruned, nil
}

// octreeSafeMove appends cubes from the end of src to dst while taking care
// not to leave dst without space to decompose to smallest cube level using DFS.
// Cubes appended to dst from src are removed from src.
func octreeSafeMove(dst, src []icube) (newDst, newSrc []icube) {
	if len(src) == 0 {
		return dst, src
	}
	// Calculate amount of cubes that would be generated in DFS
	srcGenCubes := 8 * (src[0].lvl + 1) // TODO(soypat): Checking the first cube is very (read as "too") conservative.
	neededSpace := 1 + srcGenCubes      // plus one for appended cube.
	// Calculate free space in dst after cubes generated by 1 decomposition+append.
	free := cap(dst) - neededSpace
	trimIdx := max(0, len(src)-free)
	prevCap := cap(dst)
	dst = append(dst, src[trimIdx:]...)
	if cap(dst) != prevCap {
		panic("heapless assumption broken")
	}
	src = src[:trimIdx]
	return dst, src
}

func octreeSafeSpread(dstWithLvl0, src []icube, numLvl0 int) (newDst, newSrc []icube, newNumLvl0 int) {
	if len(src) == 0 || numLvl0 == 0 || len(dstWithLvl0) == 0 {
		return dstWithLvl0, src, numLvl0 // No work to do.
	}
	srcIdx := len(src) - 1 // Start appending from end of src.
	cube := src[srcIdx]
	neededSpace := 8*cube.lvl + 1
	for i := 0; numLvl0 > 0 && i < len(dstWithLvl0); i++ {
		free := cap(dstWithLvl0) - i
		if free < neededSpace {
			break // If we add this cube we'd overflow the target buffer upon DFS decomposition, so don't.
		}
		// Look for zero level cubes (invalid/empty/discarded).
		if dstWithLvl0[i].lvl != 0 {
			continue
		} else if cube.lvl == 0 {
			panic("bad src cube in octreeSafeSpread")
		}
		// Calculate free space.
		dstWithLvl0[i] = cube
		numLvl0--
		srcIdx--
		if srcIdx < 0 {
			break // Done processing cubes.
		}
		cube = src[srcIdx]
		neededSpace = 8*cube.lvl + 1
	}
	return dstWithLvl0, src[:srcIdx+1], numLvl0
}
