package glrender

import (
	"errors"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/gleval"
)

// dualRender dual contouring implementation.
func dualRender(dst []ms3.Triangle, sdf gleval.SDF3, res float32) ([]ms3.Triangle, error) {
	bb := sdf.Bounds() //.Add(ms3.Vec{X: -res / 2, Y: -res / 2, Z: -res / 2})
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
	edgeInfo := make([]dualEdgeInfo, iCubes)
	for j := 0; j < iCubes; j++ {
		edgeInfo[j] = makeDualCubeInfo(distbuf[j*4:])
	}

	return dst, nil
}

// minecraftRenderer performs a minecraft-like render of the SDF using a dual contour method.
// Appends rendered triangles to dst and returning the result.
func minecraftRenderer(dst []ms3.Triangle, sdf gleval.SDF3, res float32) ([]ms3.Triangle, error) {
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
		dci := makeDualCubeInfo(distbuf[j*4:])
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

func makeDualCubeInfo(data []float32) dualEdgeInfo {
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
