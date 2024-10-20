package glrender

import (
	"errors"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/gleval"
)

type DualContourRenderer struct {
	sdf     gleval.SDF3
	cubeMap map[ivec]int
	cubebuf []icube
	posbuf  []ms3.Vec
	distbuf []float32
	// cubeinfo stores both cube and edge information for dual contouring algorithm.
	// Edge information corresponds to the edges that coincide in the cube origin.
	cubeinfo []dualCube
	res      float32
	origin   ms3.Vec
}

type DualContourConfig struct {
}

func (dcr *DualContourRenderer) Reset(sdf gleval.SDF3, userData any, res float32, cfg DualContourConfig) error {
	bbSub := res / 2
	bb := sdf.Bounds().Add(ms3.Vec{X: -bbSub, Y: -bbSub, Z: -bbSub})
	// bb.Min = ms3.Sub(bb.Min, ms3.Vec{X: bbSub, Y: bbSub, Z: bbSub})
	topCube, origin, err := makeICube(bb, res)
	if err != nil {
		return err
	}
	nCubes := int(topCube.decomposesTo(1))
	if cap(dcr.cubebuf) < nCubes*2 {
		dcr.cubebuf = make([]icube, 0, nCubes*2) // Want to fully decompose cube.
	}
	var ok bool
	dcr.cubebuf, ok = octreeDecomposeBFS(dcr.cubebuf[:0], topCube, 1)
	if !ok {
		return errors.New("unable to decompose top level cube")
	} else if dcr.cubebuf[0].lvl != 1 {
		return errors.New("short buffer decomposing all cubes")
	} else if len(dcr.cubebuf) != nCubes {
		panic("failed to decompose all edges?")
	}
	if cap(dcr.posbuf) < nCubes {
		dcr.posbuf = make([]ms3.Vec, nCubes)
		dcr.distbuf = make([]float32, nCubes)
	}
	dcr.posbuf = dcr.posbuf[:0]
	posbuf := dcr.posbuf[:cap(dcr.posbuf)]

	// Keep cubes that contain a surface and their neighbors.
	dcr.cubebuf, _, err = octreePrune(sdf, dcr.cubebuf, origin, res, posbuf, dcr.distbuf[:len(posbuf)], userData, 2, true)
	if err != nil {
		return err
	}
	// New number of cubes due to reduction by pruning.
	nCubes = len(dcr.cubebuf)
	if cap(dcr.cubeinfo) < nCubes {
		dcr.cubeinfo = make([]dualCube, nCubes)
	}
	if dcr.cubeMap == nil {
		dcr.cubeMap = make(map[ivec]int)
	}
	clear(dcr.cubeMap)
	dcr.res = res
	dcr.origin = origin
	dcr.sdf = sdf
	return nil
}

func (dcr *DualContourRenderer) RenderAll(dst []ms3.Triangle, userData any) ([]ms3.Triangle, error) {
	cubeMap := dcr.cubeMap
	edges := dcr.cubebuf
	res := dcr.res
	origin := dcr.origin
	posbuf := dcr.posbuf[:0]
	sdf := dcr.sdf

	for e, edge := range edges {
		sz := edge.size(res)
		edgeOrig := edge.origin(origin, sz)
		// posbuf has edge origin, edge extremes in x,y,z and the center of the voxel.
		posbuf = append(posbuf,
			edgeOrig,
			ms3.Add(edgeOrig, ms3.Vec{X: sz}),
			ms3.Add(edgeOrig, ms3.Vec{Y: sz}),
			ms3.Add(edgeOrig, ms3.Vec{Z: sz}),
			edge.center(origin, sz),
		)
		cubeMap[edge.ivec] = e
	}

	lenPos := len(posbuf)
	distbuf := make([]float32, lenPos)
	err := sdf.Evaluate(posbuf, distbuf, nil)
	if err != nil {
		return dst, err
	}
	// posbuf will contain edge intersection position.
	posbuf = posbuf[:0]
	cubes := dcr.cubeinfo[:len(edges)]
	for e, edge := range edges {
		// First for loop accumulates edge biases into voxels/cubes.
		cube := makeDualCube(edge.ivec, distbuf[e*5:])
		if !cube.isActive() {
			continue
		}
		cubes[e] = cube
		sz := edge.size(res)
		edgeOrigin := edge.origin(origin, sz)
		if cube.xActive() {
			t := cube.xIsectLinear()
			x := ms3.Add(edgeOrigin, ms3.Vec{X: sz * t})
			posbuf = append(posbuf, x)
			idx := len(posbuf) - 1
			for _, iv := range dualEdgeCubeNeighbors(edge.ivec, 0) {
				cubes[cubeMap[iv]].addBiasVert(x, idx)
			}
		}
		if cube.yActive() {
			t := cube.yIsectLinear()
			y := ms3.Add(edgeOrigin, ms3.Vec{Y: sz * t})
			posbuf = append(posbuf, y)
			idx := len(posbuf) - 1
			for _, iv := range dualEdgeCubeNeighbors(edge.ivec, 1) {
				cubes[cubeMap[iv]].addBiasVert(y, idx)
			}
		}
		if cube.zActive() {
			t := cube.zIsectLinear()
			z := ms3.Add(edgeOrigin, ms3.Vec{Z: sz * t})
			posbuf = append(posbuf, z)
			idx := len(posbuf) - 1
			for _, iv := range dualEdgeCubeNeighbors(edge.ivec, 2) {
				cubes[cubeMap[iv]].addBiasVert(z, idx)
			}
		}
	}

	normals := make([]ms3.Vec, len(posbuf))
	const normStep = 2e-5
	err = gleval.NormalsCentralDiff(sdf, posbuf, normals, normStep, userData)
	if err != nil {
		return dst, err
	}
	for e, dc := range cubes {
		if len(dc.BiasVerts) == 0 {
			continue
		}
		edge := edges[e]
		sz := edge.size(res)
		cubeOrigin := edge.origin(origin, sz)

		// Initialize AtA and Atb
		var AtA ms3.Mat3
		var Atb ms3.Vec
		// For each bias vert and corresponding normal
		for i := 0; i < len(dc.BiasVerts); i++ {
			pi := dc.BiasVerts[i]
			qi := ms3.Sub(pi, cubeOrigin) // Local coordinates within the cube
			ni := normals[dc.BiasVertIdxs[i]]
			ni = ms3.Unit(ni)
			// Compute outer product ni * ni^T
			outer := ms3.Prod(ni, ni)
			AtA = ms3.AddMat3(AtA, outer)
			// Compute ni * (ni^T * qi)
			dot := ms3.Dot(ni, qi)
			scaledNi := ms3.Scale(dot, ni)
			Atb = ms3.Add(Atb, scaledNi)
		}
		bias := dc.vertMean()
		// Regularization to handle singular matrices
		lambda := float32(3e-3)
		AtA = ms3.AddMat3(AtA, ms3.ScaleMat3(ms3.IdentityMat3(), lambda))
		Atb = ms3.Add(Atb, ms3.Scale(lambda, ms3.Sub(bias, cubeOrigin)))
		// Solve AtA x = Atb
		det := AtA.Determinant()
		if math32.Abs(det) < 1e-5 {
			// Singular or near-singular matrix; fall back to mean position
			cubes[e].FinalVertex = bias
			// dc.vert = vert
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

	var quads [][4]ms3.Vec
	for e := range edges {
		// Loop over edges once all biases have been accumulated into cubes.
		cube := cubes[e]
		if !cube.isActive() {
			continue
		}
		var quad [4]ms3.Vec
		if cube.xActive() {
			for iq, iv := range cube.cubeNeighborsToEdge(0) {
				cinfo := cubes[cubeMap[iv]]
				quad[iq] = cinfo.FinalVertex
			}
			if cube.xFlip() {
				quad = [4]ms3.Vec{quad[3], quad[2], quad[1], quad[0]}
			}
			quads = append(quads, quad)
		}
		if cube.yActive() {
			for iq, iv := range cube.cubeNeighborsToEdge(1) {
				cinfo := cubes[cubeMap[iv]]
				quad[iq] = cinfo.FinalVertex
			}
			if cube.yFlip() {
				quad = [4]ms3.Vec{quad[3], quad[2], quad[1], quad[0]}
			}
			quads = append(quads, quad)
		}
		if cube.zActive() {
			for iq, iv := range cube.cubeNeighborsToEdge(2) {
				cinfo := cubes[cubeMap[iv]]
				quad[iq] = cinfo.FinalVertex
			}
			if cube.zFlip() {
				quad = [4]ms3.Vec{quad[3], quad[2], quad[1], quad[0]}
			}
			quads = append(quads, quad)
		}
	}
	for _, q := range quads {
		dst = append(dst,
			ms3.Triangle{q[0], q[1], q[2]},
			ms3.Triangle{q[2], q[3], q[0]},
		)
	}
	return dst, nil

}

func makeDualCube(ivec ivec, data []float32) dualCube {
	if len(data) < 4 {
		panic("short dual cube info buffer")
	}
	return dualCube{
		OrigDist: data[0],
		XDist:    data[1],
		YDist:    data[2],
		ZDist:    data[3],
		ivec:     ivec,
	}
}

type dualCube struct {
	// Distance from cube origin to SDF.
	OrigDist float32
	// Distance from (x,y,z) edge vertices to SDF.
	XDist, YDist, ZDist float32
	BiasVerts           []ms3.Vec
	BiasVertIdxs        []int
	// FinalVertex set by vertex strategy. Decides the final resting place of the vertex of the cube
	// which will be the vertex meshed.
	FinalVertex ms3.Vec
	ivec        ivec
}

func (dc *dualCube) addBiasVert(v ms3.Vec, idx int) {
	dc.BiasVerts = append(dc.BiasVerts, v)
	dc.BiasVertIdxs = append(dc.BiasVertIdxs, idx)
}

func (dc *dualCube) vertMean() (mean ms3.Vec) {
	for i := 0; i < len(dc.BiasVerts); i++ {
		mean = ms3.Add(mean, dc.BiasVerts[i])
	}
	return ms3.Scale(1./float32(len(dc.BiasVerts)), mean)
}

func (dc *dualCube) cubeNeighborsToEdge(axis int) [4]ivec {
	v := dc.ivec
	const sub = -2
	switch axis {
	case 0: // x
		return [4]ivec{}
	case 1: // y
		return [4]ivec{
			v.Add(ivec{x: sub, z: sub}), v.Add(ivec{x: sub}), v, v.Add(ivec{z: sub}),
		}
	case 2: // z
		return [4]ivec{
			v.Add(ivec{x: sub, y: sub}), v.Add(ivec{y: sub}), v, v.Add(ivec{x: sub}),
		}
	}
	panic("invalid axis")
}

func (dc *dualCube) isActive() bool {
	return dc.xActive() || dc.yActive() || dc.zActive()
}

func (dc *dualCube) xActive() bool {
	return math32.Float32bits(dc.OrigDist)&(1<<31) != math32.Float32bits(dc.XDist)&(1<<31) // Sign bit differs.
}
func (dc *dualCube) yActive() bool {
	return math32.Float32bits(dc.OrigDist)&(1<<31) != math32.Float32bits(dc.YDist)&(1<<31)
}
func (dc *dualCube) zActive() bool {
	return math32.Float32bits(dc.OrigDist)&(1<<31) != math32.Float32bits(dc.ZDist)&(1<<31)
}
func (dc *dualCube) xIsectLinear() float32 { return -dc.OrigDist / (dc.XDist - dc.OrigDist) }
func (dc *dualCube) yIsectLinear() float32 { return -dc.OrigDist / (dc.YDist - dc.OrigDist) }
func (dc *dualCube) zIsectLinear() float32 { return -dc.OrigDist / (dc.ZDist - dc.OrigDist) }
func (dc *dualCube) xFlip() bool           { return dc.XDist-dc.OrigDist < 0 }
func (dc *dualCube) yFlip() bool           { return dc.YDist-dc.OrigDist < 0 }
func (dc *dualCube) zFlip() bool           { return dc.ZDist-dc.OrigDist < 0 }

func dualEdgeCubeNeighbors(v ivec, axis int) [4]ivec {
	const sub = -2
	switch axis {
	case 0: // x
		return [4]ivec{
			v.Add(ivec{y: sub, z: sub}), v.Add(ivec{z: sub}), v, v.Add(ivec{y: sub}),
		}
	case 1: // y
		return [4]ivec{
			v.Add(ivec{x: sub, z: sub}), v.Add(ivec{x: sub}), v, v.Add(ivec{z: sub}),
		}
	case 2: // z
		return [4]ivec{
			v.Add(ivec{x: sub, y: sub}), v.Add(ivec{y: sub}), v, v.Add(ivec{x: sub}),
		}
	}
	panic("invalid axis")
}

// minecraftRender performs a minecraft-like render of the SDF using a dual contour method.
// Appends rendered triangles to dst and returning the result.
func minecraftRender(dst []ms3.Triangle, sdf gleval.SDF3, res float32) ([]ms3.Triangle, error) {
	bb := sdf.Bounds()
	topCube, origin, err := makeICube(bb, res)
	if err != nil {
		return dst, err
	}
	decomp := topCube.decomposesTo(1)
	cubes := make([]icube, 0, decomp*2)
	cubes, ok := octreeDecomposeBFS(cubes, topCube, 1)
	if !ok {
		return dst, errors.New("unable to decompose top level cube")
	} else if cubes[0].lvl != 1 {
		return dst, errors.New("short buffer decomposing all cubes")
	}

	var posbuf []ms3.Vec
	iCubes := 0
	for ; iCubes < len(cubes); iCubes++ {
		cube := cubes[iCubes]
		sz := cube.size(res)
		cubeOrig := cube.origin(origin, sz)
		// Append origin and edge-end vertices.
		posbuf = append(posbuf,
			cubeOrig,
			ms3.Add(cubeOrig, ms3.Vec{X: sz}),
			ms3.Add(cubeOrig, ms3.Vec{Y: sz}),
			ms3.Add(cubeOrig, ms3.Vec{Z: sz}),
		)
	}
	lenPos := len(posbuf)
	distbuf := make([]float32, lenPos)
	err = sdf.Evaluate(posbuf, distbuf, nil)
	if err != nil {
		return dst, err
	}
	for j := 0; j < iCubes; j++ {
		cube := cubes[j]
		sz := cube.size(res)
		srcOrig := cube.origin(origin, sz)
		dci := makeDualCube(cube.ivec, distbuf[j*4:])
		origOff := sz
		if dci.xActive() {
			xOrig := ms3.Add(srcOrig, ms3.Vec{X: origOff})
			dst = append(dst,
				ms3.Triangle{
					xOrig,
					ms3.Add(xOrig, ms3.Vec{Y: sz}),
					ms3.Add(xOrig, ms3.Vec{Y: sz, Z: sz}),
				},
				ms3.Triangle{
					ms3.Add(xOrig, ms3.Vec{Y: sz, Z: sz}),
					ms3.Add(xOrig, ms3.Vec{Z: sz}),
					xOrig,
				},
			)
			if dci.xFlip() {
				dst[len(dst)-1][0], dst[len(dst)-1][2] = dst[len(dst)-1][2], dst[len(dst)-1][0]
				dst[len(dst)-2][0], dst[len(dst)-2][2] = dst[len(dst)-2][2], dst[len(dst)-2][0]
			}
		}
		if dci.yActive() {
			yOrig := ms3.Add(srcOrig, ms3.Vec{Y: origOff})
			dst = append(dst,
				ms3.Triangle{
					yOrig,
					ms3.Add(yOrig, ms3.Vec{Z: sz}),
					ms3.Add(yOrig, ms3.Vec{Z: sz, X: sz}),
				},
				ms3.Triangle{
					ms3.Add(yOrig, ms3.Vec{Z: sz, X: sz}),
					ms3.Add(yOrig, ms3.Vec{X: sz}),
					yOrig,
				},
			)
			if dci.yFlip() {
				dst[len(dst)-1][0], dst[len(dst)-1][2] = dst[len(dst)-1][2], dst[len(dst)-1][0]
				dst[len(dst)-2][0], dst[len(dst)-2][2] = dst[len(dst)-2][2], dst[len(dst)-2][0]
			}
		}
		if dci.zActive() {
			zOrig := ms3.Add(srcOrig, ms3.Vec{Z: origOff})
			dst = append(dst,
				ms3.Triangle{
					zOrig,
					ms3.Add(zOrig, ms3.Vec{X: sz}),
					ms3.Add(zOrig, ms3.Vec{X: sz, Y: sz}),
				},
				ms3.Triangle{
					ms3.Add(zOrig, ms3.Vec{X: sz, Y: sz}),
					ms3.Add(zOrig, ms3.Vec{Y: sz}),
					zOrig,
				},
			)
			if dci.zFlip() {
				dst[len(dst)-1][0], dst[len(dst)-1][2] = dst[len(dst)-1][2], dst[len(dst)-1][0]
				dst[len(dst)-2][0], dst[len(dst)-2][2] = dst[len(dst)-2][2], dst[len(dst)-2][0]
			}
		}
	}
	return dst, nil
}
