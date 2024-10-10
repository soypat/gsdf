package glrender

import (
	"errors"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
)

// This file contains basic low level algorithms regarding Octrees.

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

// decomposeOctreeDFS decomposes icubes from the end of cubes into their octree sub-icubes
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
func decomposeOctreeDFS(dst []ms3.Vec, cubes []icube, origin ms3.Vec, res float32) ([]ms3.Vec, []icube) {
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

// decomposeOctreeBFS decomposes start into octree cubes and appends them to dst without surpassing dst's slice capacity.
// Smallest cubes will remain at the highest index of dst. The boolean value returned indicates whether the
// argument start icube was able to be decomposed and its children added to dst.
func decomposeOctreeBFS(dst []icube, start icube, minimumDecomposedLvl int) ([]icube, bool) {
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
