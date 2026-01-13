package glrender

import (
	"math"

	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/gleval"
)

type DualContourer interface {
	// PlaceVertices should edit the FinalVertex field of all [DualCube]s in the cubes buffer.
	// These resulting vertices are then used for quad/triangle meshing.
	PlaceVertices(cubes []DualCube, origin ms3.Vec, res float32, sdf gleval.SDF3, posbuf []ms3.Vec, distbuf []float32, userData any) error
}

// DualContourLeastSquares is a vertex placement strategy which solves the least squares problem
// to place vertices.
type DualContourLeastSquares struct {
	// Chiseled enables sharper feature detection at the cost of potential artifacts
	// at SDF discontinuities (e.g., union boundaries). When false (default), uses
	// settings that more closely match Python sdftoolbox behavior.
	Chiseled bool
}

func (lsq *DualContourLeastSquares) PlaceVertices(cubes []DualCube, origin ms3.Vec, res float32, sdf gleval.SDF3, posbuf []ms3.Vec, distbuf []float32, userData any) error {
	// Prepare for normal calculation.
	posbuf = posbuf[:0]
	for c := range cubes {
		cube := &cubes[c]
		sz, norig := cube.SizeAndOrigin(res, origin)
		posbuf = append(posbuf,
			ms3.Add(norig, ms3.Vec{X: sz * cube.IsectLinearX()}),
			ms3.Add(norig, ms3.Vec{Y: sz * cube.IsectLinearY()}),
			ms3.Add(norig, ms3.Vec{Z: sz * cube.IsectLinearZ()}),
		)
	}

	normals := make([]ms3.Vec, len(posbuf))
	// NormalsCentralDiff returns raw (f(x+h/2) - f(x-h/2)) without division.
	// Chiseled mode uses larger step for sharper features; default matches Python.
	var normStep float64
	if lsq.Chiseled {
		normStep = 1e-4 // Sharper features, may have artifacts at union boundaries
	} else {
		normStep = 2e-8 // Matches Python sdftoolbox gradient step
	}
	err := gleval.NormalsCentralDiff(sdf, posbuf, normals, float32(normStep), userData)
	if err != nil {
		return err
	}
	var biasVerts []ms3.Vec
	var localNormals []ms3.Vec
	// Preallocate A matrix and b vector for least squares (max 12 edges + 3 regularization = 15 rows)
	A := make([][3]float32, 0, 15)
	b := make([]float32, 0, 15)
	for e, cube := range cubes {
		if len(cube.Neighbors) == 0 {
			continue
		}
		sz, cubeOrigin := cube.SizeAndOrigin(res, origin)

		biasVerts = biasVerts[:0]
		localNormals = localNormals[:0]

		// Add the cube's OWN active edges first (Python uses all 12 edges of a voxel).
		// The cube owns 3 edges at its origin corner.
		if cube.ActiveX() {
			biasVerts = append(biasVerts, ms3.Add(cubeOrigin, ms3.Vec{X: sz * cube.IsectLinearX()}))
			localNormals = append(localNormals, normals[3*e+0])
		}
		if cube.ActiveY() {
			biasVerts = append(biasVerts, ms3.Add(cubeOrigin, ms3.Vec{Y: sz * cube.IsectLinearY()}))
			localNormals = append(localNormals, normals[3*e+1])
		}
		if cube.ActiveZ() {
			biasVerts = append(biasVerts, ms3.Add(cubeOrigin, ms3.Vec{Z: sz * cube.IsectLinearZ()}))
			localNormals = append(localNormals, normals[3*e+2])
		}

		// Add edges from neighboring cubes (the other 9 edges of this voxel).
		for _, n := range cube.Neighbors {
			neighbor := cubes[n[0]]
			axis := n[1]
			nsz, norig := neighbor.SizeAndOrigin(res, origin)
			var contrib ms3.Vec
			switch axis {
			case 0:
				contrib = ms3.Add(norig, ms3.Vec{X: nsz * neighbor.IsectLinearX()})
			case 1:
				contrib = ms3.Add(norig, ms3.Vec{Y: nsz * neighbor.IsectLinearY()})
			case 2:
				contrib = ms3.Add(norig, ms3.Vec{Z: nsz * neighbor.IsectLinearZ()})
			}
			biasVerts = append(biasVerts, contrib)
			localNormals = append(localNormals, normals[3*n[0]+axis])
		}

		// Build A matrix and b vector for least squares Ax = b
		// Work in normalized [0,1) voxel coordinates like Python sdftoolbox.
		invRes := 1.0 / res
		A = A[:0]
		b = b[:0]
		for i := 0; i < len(biasVerts); i++ {
			pi := biasVerts[i]
			// qi in [0,1) normalized coordinates within the voxel
			qi := ms3.Scale(invRes, ms3.Sub(pi, cubeOrigin))
			ni := localNormals[i]
			// Each row of A is the normal, b[i] = n^T * q
			A = append(A, [3]float32{ni.X, ni.Y, ni.Z})
			b = append(b, ms3.Dot(ni, qi))
		}

		// Add regularization rows. Python uses bias_strength=1e-5 with unit normals.
		bias := ms3.Scale(invRes, ms3.Sub(vertMean(biasVerts), cubeOrigin))
		var sqrtLambda float32
		if lsq.Chiseled {
			// Scale lambda to match normal magnitude: sqrtLambda = sqrt(1e-5) * normStep
			sqrtLambda = float32(math.Sqrt(1e-5) * normStep)
		} else {
			// Match Python exactly
			sqrtLambda = float32(math.Sqrt(1e-5))
		}
		A = append(A, [3]float32{sqrtLambda, 0, 0})
		A = append(A, [3]float32{0, sqrtLambda, 0})
		A = append(A, [3]float32{0, 0, sqrtLambda})
		b = append(b, sqrtLambda*bias.X)
		b = append(b, sqrtLambda*bias.Y)
		b = append(b, sqrtLambda*bias.Z)

		// Solve using Modified Gram-Schmidt QR with float64 precision
		// (matching Python's np.linalg.lstsq(A.astype(float), b.astype(float))).
		xArr := leastSquaresMGS64(A, b)
		x := ms3.Vec{X: xArr[0], Y: xArr[1], Z: xArr[2]}
		// Clip to voxel bounds with 10% relaxation (matching Python vertex_relaxation_percent=0.1).
		x = ms3.ClampElem(x, ms3.Vec{X: -0.1, Y: -0.1, Z: -0.1}, ms3.Vec{X: 1.1, Y: 1.1, Z: 1.1})
		// Convert back from [0,1) to world coordinates.
		vert := ms3.Add(ms3.Scale(res, x), cubeOrigin)
		cubes[e].FinalVertex = vert
	}
	return nil
}

func vertMean(verts []ms3.Vec) (mean ms3.Vec) {
	for i := 0; i < len(verts); i++ {
		mean = ms3.Add(mean, verts[i])
	}
	return ms3.Scale(1./float32(len(verts)), mean)
}

// leastSquaresMGS64 solves the overdetermined system Ax = b using Modified Gram-Schmidt QR
// with float64 precision, matching Python's np.linalg.lstsq(A.astype(float), b.astype(float)).
// A is K×3 (normals), b is K×1.
func leastSquaresMGS64(A [][3]float32, b []float32) [3]float32 {
	K := len(A)
	if K < 3 {
		return [3]float32{}
	}

	// Convert to float64 for precision (matching Python)
	Q := make([][3]float64, K)
	for k := 0; k < K; k++ {
		Q[k] = [3]float64{float64(A[k][0]), float64(A[k][1]), float64(A[k][2])}
	}
	b64 := make([]float64, K)
	for k := 0; k < K; k++ {
		b64[k] = float64(b[k])
	}

	// R is 3×3 upper triangular
	var R [3][3]float64

	// Modified Gram-Schmidt: orthogonalize columns
	for j := 0; j < 3; j++ {
		for i := 0; i < j; i++ {
			var dot float64
			for k := 0; k < K; k++ {
				dot += Q[k][i] * Q[k][j]
			}
			R[i][j] = dot
			for k := 0; k < K; k++ {
				Q[k][j] -= dot * Q[k][i]
			}
		}

		var normSq float64
		for k := 0; k < K; k++ {
			normSq += Q[k][j] * Q[k][j]
		}
		norm := math.Sqrt(normSq)
		R[j][j] = norm

		if norm > 1e-14 {
			invNorm := 1.0 / norm
			for k := 0; k < K; k++ {
				Q[k][j] *= invNorm
			}
		}
	}

	// Compute Q^T * b
	var Qtb [3]float64
	for j := 0; j < 3; j++ {
		for k := 0; k < K; k++ {
			Qtb[j] += Q[k][j] * b64[k]
		}
	}

	// Back-substitution: solve Rx = Q^T b
	var x [3]float64
	for i := 2; i >= 0; i-- {
		x[i] = Qtb[i]
		for k := i + 1; k < 3; k++ {
			x[i] -= R[i][k] * x[k]
		}
		if R[i][i] > 1e-14 {
			x[i] /= R[i][i]
		} else {
			x[i] = 0
		}
	}

	return [3]float32{float32(x[0]), float32(x[1]), float32(x[2])}
}
