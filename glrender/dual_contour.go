package glrender

import (
	"errors"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/gleval"
)

// dualRender dual contouring implementation.
func dualRender(dst []ms3.Triangle, sdf gleval.SDF3, res float32, userData any) ([]ms3.Triangle, error) {
	bbSub := res / 2
	bb := sdf.Bounds().Add(ms3.Vec{X: -bbSub, Y: -bbSub, Z: -bbSub})
	// bb.Min = ms3.Sub(bb.Min, ms3.Vec{X: bbSub, Y: bbSub, Z: bbSub})
	topCube, origin, err := makeICube(bb, res)
	if err != nil {
		return dst, err
	}
	nEdges := topCube.decomposesTo(1)
	edges := make([]icube, 0, nEdges*2)
	edges, ok := octreeDecomposeBFS(edges, topCube, 1)
	if !ok {
		return dst, errors.New("unable to decompose top level cube")
	} else if edges[0].lvl != 1 {
		return dst, errors.New("short buffer decomposing all cubes")
	} else if len(edges) != nEdges {
		panic("failed to decompose all edges?")
	}

	var posbuf []ms3.Vec

	edgeMap := make(map[ivec]int) // Maps a icube vec to an index.
	for e := 0; e < nEdges; e++ {
		edge := edges[e]
		sz := edge.size(res)
		edgeOrig := edge.origin(origin, sz)
		posbuf = append(posbuf,
			edgeOrig,
			ms3.Add(edgeOrig, ms3.Vec{X: sz}),
			ms3.Add(edgeOrig, ms3.Vec{Y: sz}),
			ms3.Add(edgeOrig, ms3.Vec{Z: sz}),
			edge.center(origin, sz),
		)
		edgeMap[edge.ivec] = e
	}
	lenPos := len(posbuf)
	distbuf := make([]float32, lenPos)
	// normals := make([]ms3.Vec, lenPos)
	err = sdf.Evaluate(posbuf, distbuf, nil)
	if err != nil {
		return dst, err
	}
	// err = gleval.NormalsCentralDiff(sdf, posbuf, normals, res/1000, userData)
	// if err != nil {
	// 	return dst, err
	// }
	//
	edgeInfo := make([]dualEdgeInfo, nEdges)
	cubeInfo := make([]dualCubeInfo, nEdges)
	for e, edge := range edges {
		// First for loop accumulates edge biases into voxels/cubes.
		einfo := makeDualEdgeInfo(distbuf[e*5:])
		if !einfo.isActive() {
			continue
		}
		edgeInfo[e] = einfo
		sz := edge.size(res)
		edgeOrigin := edge.origin(origin, sz)
		if einfo.xActive() {
			t := einfo.xIsectLinear()
			x := ms3.Add(edgeOrigin, ms3.Vec{X: sz * t})
			for _, iv := range dualEdgeCubeNeighbors(edge.ivec, 0) {
				cubeInfo[edgeMap[iv]].addBiasVert(x)
			}
		}
		if einfo.yActive() {
			t := einfo.yIsectLinear()
			y := ms3.Add(edgeOrigin, ms3.Vec{Y: sz * t})
			for _, iv := range dualEdgeCubeNeighbors(edge.ivec, 1) {
				cubeInfo[edgeMap[iv]].addBiasVert(y)
			}
		}
		if einfo.zActive() {
			t := einfo.zIsectLinear()
			z := ms3.Add(edgeOrigin, ms3.Vec{Z: sz * t})
			for _, iv := range dualEdgeCubeNeighbors(edge.ivec, 2) {
				idx := edgeMap[iv]
				cubeInfo[idx].addBiasVert(z)
			}
		}

	}
	var quads [][4]ms3.Vec
	for e, edge := range edges {
		// Loop over edges once all biases have been accumulated into cubes.
		einfo := edgeInfo[e]
		if !einfo.isActive() {
			continue
		}
		var quad [4]ms3.Vec
		if einfo.xActive() {
			for iq, iv := range dualEdgeCubeNeighbors(edge.ivec, 0) {
				cinfo := cubeInfo[edgeMap[iv]]
				quad[iq] = cinfo.vertMean()
			}
			if einfo.xFlip() {
				quad = [4]ms3.Vec{quad[3], quad[2], quad[1], quad[0]}
			}
			quads = append(quads, quad)
		}
		if einfo.yActive() {
			for iq, iv := range dualEdgeCubeNeighbors(edge.ivec, 1) {
				quad[iq] = cubeInfo[edgeMap[iv]].vertMean()
			}
			if einfo.yFlip() {
				quad = [4]ms3.Vec{quad[3], quad[2], quad[1], quad[0]}
			}
			quads = append(quads, quad)
		}
		if einfo.zActive() {
			for iq, iv := range dualEdgeCubeNeighbors(edge.ivec, 2) {
				quad[iq] = cubeInfo[edgeMap[iv]].vertMean()
			}
			if einfo.zFlip() {
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

// minecraftRender performs a minecraft-like render of the SDF using a dual contour method.
// Appends rendered triangles to dst and returning the result.
func minecraftRender(dst []ms3.Triangle, sdf gleval.SDF3, res float32) ([]ms3.Triangle, error) {
	bb := sdf.Bounds()
	topCube, origin, err := makeICube(bb, res)
	if err != nil {
		return dst, err
	}
	decomp := topCube.decomposesTo(1)
	cubes := make([]icube, 0, decomp)
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
		dci := makeDualEdgeInfo(distbuf[j*4:])
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

type dualCubeInfo struct {
	biasVerts []ms3.Vec
	normal    ms3.Vec
	vert      ms3.Vec
}

func (dc *dualCubeInfo) addBiasVert(v ms3.Vec) {
	dc.biasVerts = append(dc.biasVerts, v)
}

func (dc dualCubeInfo) vertMean() (mean ms3.Vec) {
	for i := 0; i < len(dc.biasVerts); i++ {
		mean = ms3.Add(mean, dc.biasVerts[i])
	}
	return ms3.Scale(1./float32(len(dc.biasVerts)), mean)
}

func makeDualEdgeInfo(data []float32) dualEdgeInfo {
	if len(data) < 4 {
		panic("short dual cube info buffer")
	}
	return dualEdgeInfo{
		cSDF: data[0],
		xSDF: data[1],
		ySDF: data[2],
		zSDF: data[3],
	}
}

type dualEdgeInfo struct {
	// SDF evaluation at edge vertices.
	cSDF, xSDF, ySDF, zSDF float32
}

func (dc dualEdgeInfo) appendIsectLinearCoords(dst []ms3.Vec, edgeOrigin ms3.Vec, size float32) []ms3.Vec {
	if dc.xActive() {
		dst = append(dst, ms3.Add(edgeOrigin, ms3.Vec{X: size * dc.xIsectLinear()}))
	}
	if dc.yActive() {
		dst = append(dst, ms3.Add(edgeOrigin, ms3.Vec{Y: size * dc.yIsectLinear()}))
	}
	if dc.zActive() {
		dst = append(dst, ms3.Add(edgeOrigin, ms3.Vec{Z: size * dc.zIsectLinear()}))
	}
	return dst
}

func (dc dualEdgeInfo) xIsectLinear() float32 { return -dc.cSDF / (dc.xSDF - dc.cSDF) }
func (dc dualEdgeInfo) yIsectLinear() float32 { return -dc.cSDF / (dc.ySDF - dc.cSDF) }
func (dc dualEdgeInfo) zIsectLinear() float32 { return -dc.cSDF / (dc.zSDF - dc.cSDF) }

func (dc dualEdgeInfo) isActive() bool {
	return dc.xActive() || dc.yActive() || dc.zActive()
}

func (dc dualEdgeInfo) xActive() bool {
	return math32.Float32bits(dc.cSDF)&(1<<31) != math32.Float32bits(dc.xSDF)&(1<<31) // Sign bit differs.
}
func (dc dualEdgeInfo) yActive() bool {
	return math32.Float32bits(dc.cSDF)&(1<<31) != math32.Float32bits(dc.ySDF)&(1<<31)
}
func (dc dualEdgeInfo) zActive() bool {
	return math32.Float32bits(dc.cSDF)&(1<<31) != math32.Float32bits(dc.zSDF)&(1<<31)
}
func (dc dualEdgeInfo) xFlip() bool {
	return dc.xSDF-dc.cSDF < 0
}
func (dc dualEdgeInfo) yFlip() bool {
	return dc.ySDF-dc.cSDF < 0
}
func (dc dualEdgeInfo) zFlip() bool {
	return dc.zSDF-dc.cSDF < 0
}

func b2u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func ptrappend[T any](ptr *[]T, v T) {
	*ptr = append(*ptr, v)
}

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
