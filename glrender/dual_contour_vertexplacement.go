package glrender

import (
	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
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
	const normStep = 2e-5
	err := gleval.NormalsCentralDiff(sdf, posbuf, normals, normStep, userData)
	if err != nil {
		return err
	}
	var biasVerts []ms3.Vec
	var localNormals []ms3.Vec
	for e, cube := range cubes {
		if len(cube.Neighbors) == 0 {
			continue
		}
		_, cubeOrigin := cube.SizeAndOrigin(res, origin)

		// Initialize AtA and Atb
		var AtA ms3.Mat3
		var Atb ms3.Vec
		biasVerts = biasVerts[:0]
		localNormals = localNormals[:0]
		for _, n := range cube.Neighbors {
			neighbor := cubes[n[0]]
			axis := n[1]
			sz, norig := neighbor.SizeAndOrigin(res, origin)
			var contrib ms3.Vec
			switch axis {
			case 0:
				contrib = ms3.Add(norig, ms3.Vec{X: sz * neighbor.IsectLinearX()})
			case 1:
				contrib = ms3.Add(norig, ms3.Vec{Y: sz * neighbor.IsectLinearY()})
			case 2:
				contrib = ms3.Add(norig, ms3.Vec{Z: sz * neighbor.IsectLinearZ()})
			}
			biasVerts = append(biasVerts, contrib)
			localNormals = append(localNormals, normals[3*n[0]+axis])
		}
		// For each bias vert and corresponding normal
		for i := 0; i < len(biasVerts); i++ {
			pi := biasVerts[i]
			qi := ms3.Sub(pi, cubeOrigin) // Local coordinates within the cube
			ni := localNormals[i]
			ni = ms3.Unit(ni)
			// Compute outer product ni * ni^T
			outer := ms3.Prod(ni, ni)
			AtA = ms3.AddMat3(AtA, outer)
			// Compute ni * (ni^T * qi)
			dot := ms3.Dot(ni, qi)
			scaledNi := ms3.Scale(dot, ni)
			Atb = ms3.Add(Atb, scaledNi)
		}
		bias := vertMean(biasVerts)
		// Regularization to handle singular matrices
		lambda := float32(3e-3)
		AtA = ms3.AddMat3(AtA, ms3.ScaleMat3(ms3.IdentityMat3(), lambda))
		Atb = ms3.Add(Atb, ms3.Scale(lambda, ms3.Sub(bias, cubeOrigin)))
		// Solve AtA x = Atb
		det := AtA.Determinant()
		if math32.Abs(det) < 1e-5 {
			// Singular or near-singular matrix; fall back to mean position
			cubes[e].FinalVertex = bias
		} else {
			// U, S, _ := AtA.SVD()
			// diag := S.VecDiag()
			// UtAtb := ms3.MulMatVec(U.Transpose(), Atb)
			// sInvUtAtb := ms3.MulElem(ms3.Vec{X: 1. / diag.X, Y: 1. / diag.Y, Z: 1. / diag.Z}, UtAtb)
			// x := ms3.MulMatVec(U, sInvUtAtb)
			AtAInv := AtA.Inverse()
			x := ms3.MulMatVec(AtAInv, Atb)
			// x = ms3.ClampElem(x, ms3.Vec{}, ms3.Vec{X: sz, Y: sz, Z: sz}) // Limit vertex to be within voxel.
			vert := ms3.Add(x, cubeOrigin) // Convert back to global coordinates
			cubes[e].FinalVertex = vert
		}
	}
	return nil
}

func vertMean(verts []ms3.Vec) (mean ms3.Vec) {
	for i := 0; i < len(verts); i++ {
		mean = ms3.Add(mean, verts[i])
	}
	return ms3.Scale(1./float32(len(verts)), mean)
}
