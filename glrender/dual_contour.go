package glrender

import (
	"errors"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/gleval"
)

type DualContourRenderer struct {
	sdf       gleval.SDF3
	contourer DualContourer
	cubeMap   map[ivec]int
	cubebuf   []icube
	posbuf    []ms3.Vec
	distbuf   []float32
	// cubeinfo stores both cube and edge information for dual contouring algorithm.
	// Edge information corresponds to the edges that coincide in the cube origin.
	cubeinfo []DualCube
	res      float32
	origin   ms3.Vec
}

func (dcr *DualContourRenderer) Reset(sdf gleval.SDF3, res float32, vertexPlacer DualContourer, userData any) error {
	if vertexPlacer == nil {
		return errors.New("nil DualContourer argument to Reset")
	}
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
		dcr.cubeinfo = make([]DualCube, nCubes)
	}
	if dcr.cubeMap == nil {
		dcr.cubeMap = make(map[ivec]int)
	}
	clear(dcr.cubeMap)
	dcr.res = res
	dcr.origin = origin
	dcr.sdf = sdf
	dcr.contourer = vertexPlacer
	return nil
}

func (dcr *DualContourRenderer) RenderAll(dst []ms3.Triangle, userData any) ([]ms3.Triangle, error) {
	cubeMap := dcr.cubeMap
	edges := dcr.cubebuf
	res := dcr.res
	origin := dcr.origin
	posbuf := dcr.posbuf[:0]
	sdf := dcr.sdf
	cubes := dcr.cubeinfo[:len(edges)]

	for e, edge := range edges {
		sz := edge.size(res)
		edgeOrig := edge.origin(origin, sz)
		// posbuf has edge origin, edge extremes in x,y,z and the center of the voxel.
		posbuf = append(posbuf,
			edgeOrig,
			ms3.Add(edgeOrig, ms3.Vec{X: sz}),
			ms3.Add(edgeOrig, ms3.Vec{Y: sz}),
			ms3.Add(edgeOrig, ms3.Vec{Z: sz}),
		)
		cubes[e].FinalVertex = edgeOrig // By default set to center.
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
	for e, edge := range edges {
		// First for loop accumulates edge biases into voxels/cubes.
		cube := makeDualCube(edge.ivec, distbuf[e*4:])
		cubes[e] = cube
		if !cube.IsActive() {
			continue
		}
		if cube.ActiveX() {
			for _, iv := range cube.EdgeNeighborsX() {
				cubes[cubeMap[iv]].Neighbors = append(cubes[cubeMap[iv]].Neighbors, [3]int{e, 0, -1})
			}
		}
		if cube.ActiveY() {
			for _, iv := range cube.EdgeNeighborsY() {
				cubes[cubeMap[iv]].Neighbors = append(cubes[cubeMap[iv]].Neighbors, [3]int{e, 1, -1})
			}
		}
		if cube.ActiveZ() {
			for _, iv := range cube.EdgeNeighborsZ() {
				cubes[cubeMap[iv]].Neighbors = append(cubes[cubeMap[iv]].Neighbors, [3]int{e, 2, -1})
			}
		}
	}
	err = dcr.contourer.PlaceVertices(cubes, origin, res, sdf, posbuf, distbuf, userData)
	if err != nil {
		return nil, err
	}
	var quads [][4]ms3.Vec
	for _, cube := range cubes {
		// Loop over edges once all biases have been accumulated into cubes.
		if !cube.IsActive() {
			continue
		}
		var quad [4]ms3.Vec
		if cube.ActiveX() {
			for iq, iv := range cube.EdgeNeighborsX() {
				cinfo := cubes[cubeMap[iv]]
				quad[iq] = cinfo.FinalVertex
			}
			if cube.FlipX() {
				quad = [4]ms3.Vec{quad[3], quad[2], quad[1], quad[0]}
			}
			quads = append(quads, quad)
		}
		if cube.ActiveY() {
			for iq, iv := range cube.EdgeNeighborsY() {
				cinfo := cubes[cubeMap[iv]]
				quad[iq] = cinfo.FinalVertex
			}
			if cube.FlipY() {
				quad = [4]ms3.Vec{quad[3], quad[2], quad[1], quad[0]}
			}
			quads = append(quads, quad)
		}
		if cube.ActiveZ() {
			for iq, iv := range cube.EdgeNeighborsZ() {
				cinfo := cubes[cubeMap[iv]]
				quad[iq] = cinfo.FinalVertex
			}
			if cube.FlipZ() {
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

func makeDualCube(ivec ivec, data []float32) DualCube {
	if len(data) < 4 {
		panic("short dual cube info buffer")
	}
	return DualCube{
		OrigDist: data[0],
		XDist:    data[1],
		YDist:    data[2],
		ZDist:    data[3],
		ivec:     ivec,
	}
}

// DualCube corresponds to a voxel anmd contains both cube and edge data.
type DualCube struct {
	// ivec stores the octree index of the cube, used to find neighboring cube ivec indices and the absolute position of the cube.
	ivec ivec
	// Neighbors contains neighboring index into dualCube buffer and contributing edge intersect axis.
	//  - Neighbors[0]: Index into dualCube buffer to cube neighbor with edge.
	//  - Neighbors[1]: Intersecting axis. 0 is x; 1 is y; 2 is z.
	//  - Neighbors[2]: Auxiliary index for use by user, such as normal indexing.
	Neighbors [][3]int
	// Distance from cube origin to SDF.
	OrigDist float32
	// Distance from (x,y,z) edge vertices to SDF. These edges are coincident with cube origin.
	XDist, YDist, ZDist float32
	// FinalVertex set by vertex placement strategy. Decides the final resting place of the vertex of the cube
	// which will be the vertex meshed.
	FinalVertex ms3.Vec
}

func (dc *DualCube) SizeAndOrigin(res float32, octreeOrigin ms3.Vec) (float32, ms3.Vec) {
	return res, icube{ivec: dc.ivec, lvl: 1}.origin(octreeOrigin, res)
}

func (dc *DualCube) IsActive() bool {
	return dc.ActiveX() || dc.ActiveY() || dc.ActiveZ()
}

func (dc *DualCube) ActiveX() bool {
	return math32.Float32bits(dc.OrigDist)&(1<<31) != math32.Float32bits(dc.XDist)&(1<<31) // Sign bit differs.
}
func (dc *DualCube) ActiveY() bool {
	return math32.Float32bits(dc.OrigDist)&(1<<31) != math32.Float32bits(dc.YDist)&(1<<31)
}
func (dc *DualCube) ActiveZ() bool {
	return math32.Float32bits(dc.OrigDist)&(1<<31) != math32.Float32bits(dc.ZDist)&(1<<31)
}
func (dc *DualCube) IsectLinearX() float32 { return -dc.OrigDist / (dc.XDist - dc.OrigDist) }
func (dc *DualCube) IsectLinearY() float32 { return -dc.OrigDist / (dc.YDist - dc.OrigDist) }
func (dc *DualCube) IsectLinearZ() float32 { return -dc.OrigDist / (dc.ZDist - dc.OrigDist) }
func (dc *DualCube) FlipX() bool           { return dc.XDist-dc.OrigDist < 0 }
func (dc *DualCube) FlipY() bool           { return dc.YDist-dc.OrigDist < 0 }
func (dc *DualCube) FlipZ() bool           { return dc.ZDist-dc.OrigDist < 0 }

func (dc *DualCube) EdgeNeighborsX() [4]ivec {
	const sub = -(1 << minIcubeLvl)
	v := dc.ivec
	return [4]ivec{v.Add(ivec{y: sub, z: sub}), v.Add(ivec{z: sub}), v, v.Add(ivec{y: sub})}
}

func (dc *DualCube) EdgeNeighborsY() [4]ivec {
	const sub = -(1 << minIcubeLvl)
	v := dc.ivec
	return [4]ivec{v.Add(ivec{x: sub, z: sub}), v.Add(ivec{x: sub}), v, v.Add(ivec{z: sub})}
}

func (dc *DualCube) EdgeNeighborsZ() [4]ivec {
	const sub = -(1 << minIcubeLvl)
	v := dc.ivec
	return [4]ivec{v.Add(ivec{x: sub, y: sub}), v.Add(ivec{y: sub}), v, v.Add(ivec{x: sub})}
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
		if dci.ActiveX() {
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
			if dci.FlipX() {
				dst[len(dst)-1][0], dst[len(dst)-1][2] = dst[len(dst)-1][2], dst[len(dst)-1][0]
				dst[len(dst)-2][0], dst[len(dst)-2][2] = dst[len(dst)-2][2], dst[len(dst)-2][0]
			}
		}
		if dci.ActiveY() {
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
			if dci.FlipY() {
				dst[len(dst)-1][0], dst[len(dst)-1][2] = dst[len(dst)-1][2], dst[len(dst)-1][0]
				dst[len(dst)-2][0], dst[len(dst)-2][2] = dst[len(dst)-2][2], dst[len(dst)-2][0]
			}
		}
		if dci.ActiveZ() {
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
			if dci.FlipZ() {
				dst[len(dst)-1][0], dst[len(dst)-1][2] = dst[len(dst)-1][2], dst[len(dst)-1][0]
				dst[len(dst)-2][0], dst[len(dst)-2][2] = dst[len(dst)-2][2], dst[len(dst)-2][0]
			}
		}
	}
	return dst, nil
}
