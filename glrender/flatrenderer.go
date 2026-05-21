package glrender

import (
	"errors"
	"io"
	"sync"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/gleval"
)

// FlatRenderer evaluates all SDF grid corners exactly once and runs marching cubes
// over the resulting flat 3D array. Each unique corner is evaluated once regardless
// of how many cubes share it, reducing evaluation count ~6x compared to the octree
// renderer for typical dense models.
type FlatRenderer struct {
	s           gleval.SDF3
	res         float32
	origin      ms3.Vec
	nx, ny, nz  int
	numParallel int
	// grid stores SDF distances at all (nx+1)*(ny+1)*(nz+1) corners.
	grid []float32
	// posbuf/distbuf are the single-goroutine evaluation buffers, freed after init.
	posbuf  []ms3.Vec
	distbuf []float32
	// cubeIdx is the current marching iteration state.
	cubeIdx     int
	initialized bool
	evaluations uint64
}

// Reset reinitializes the FlatRenderer for a new SDF and resolution, reusing
// existing buffer allocations where possible to avoid garbage.
// numParallel sets the number of goroutines used during grid evaluation; 1 means serial.
func (fr *FlatRenderer) Reset(s gleval.SDF3, cubeResolution float32, evalBufferSize, numParallel int) error {
	if cubeResolution <= 0 {
		return errors.New("invalid renderer cube resolution")
	}
	if evalBufferSize < 8 {
		return errors.New("flat renderer eval buffer size must be at least 8")
	}
	if numParallel < 1 {
		return errors.New("flat renderer numParallel must be at least 1")
	}
	bb := s.Bounds()
	bb = bb.ScaleCentered(ms3.Vec{X: 1.01, Y: 1.01, Z: 1.01})
	sz := bb.Size()
	nx := int(math32.Ceil(sz.X / cubeResolution))
	ny := int(math32.Ceil(sz.Y / cubeResolution))
	nz := int(math32.Ceil(sz.Z / cubeResolution))
	if nx <= 0 || ny <= 0 || nz <= 0 {
		return errors.New("resolution not fine enough for marching cubes")
	}
	gridSize := (nx + 1) * (ny + 1) * (nz + 1)

	grid := fr.grid
	if cap(grid) < gridSize {
		grid = make([]float32, gridSize)
	}
	posbuf := fr.posbuf
	distbuf := fr.distbuf
	if cap(posbuf) < evalBufferSize {
		posbuf = make([]ms3.Vec, evalBufferSize)
		distbuf = make([]float32, evalBufferSize)
	}

	*fr = FlatRenderer{
		s:           s,
		res:         cubeResolution,
		origin:      bb.Min,
		nx:          nx,
		ny:          ny,
		nz:          nz,
		numParallel: numParallel,
		grid:        grid[:gridSize],
		posbuf:      posbuf[:evalBufferSize],
		distbuf:     distbuf[:evalBufferSize],
	}
	return nil
}

// NewFlatRenderer creates a FlatRenderer for the given SDF at the given resolution.
// evalBufferSize controls how many positions are batched per SDF evaluation call.
// numParallel is the number of goroutines used during grid evaluation; 1 means serial.
// Use Reset to reinitialize with a new SDF without reallocating.
func NewFlatRenderer(s gleval.SDF3, cubeResolution float32, evalBufferSize, numParallel int) (*FlatRenderer, error) {
	var fr FlatRenderer
	if err := fr.Reset(s, cubeResolution, evalBufferSize, numParallel); err != nil {
		return nil, err
	}
	return &fr, nil
}

// Evaluations returns the number of SDF evaluations performed during grid initialization.
func (fr *FlatRenderer) Evaluations() uint64 {
	return fr.evaluations
}

// evalGrid dispatches grid evaluation over numParallel goroutines, splitting work
// by k-layers so each goroutine writes to a contiguous, non-overlapping region of fr.grid.
func (fr *FlatRenderer) evalGrid(userData any) error {
	if fr.numParallel <= 1 {
		n, err := fr.evalKRange(0, fr.nz+1, fr.posbuf, fr.distbuf, userData)
		fr.evaluations += n
		return err
	}

	// Clamp goroutine count: no point spawning more than there are k-planes.
	numG := fr.numParallel
	if numG > fr.nz+1 {
		numG = fr.nz + 1
	}
	errs := make([]error, numG)
	counts := make([]uint64, numG)
	bufSize := len(fr.posbuf)
	var wg sync.WaitGroup
	wg.Add(numG)
	for g := 0; g < numG; g++ {
		kStart := g * (fr.nz + 1) / numG
		kEnd := (g + 1) * (fr.nz + 1) / numG
		go func(g, kStart, kEnd int) {
			defer wg.Done()
			posbuf := make([]ms3.Vec, bufSize)
			distbuf := make([]float32, bufSize)
			var vp gleval.VecPool // independent pool per goroutine; never shared
			counts[g], errs[g] = fr.evalKRange(kStart, kEnd, posbuf, distbuf, &vp)
		}(g, kStart, kEnd)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	for _, n := range counts {
		fr.evaluations += n
	}
	return nil
}

// evalKRange evaluates grid corners for k in [kStart, kEnd) using the provided
// buffers and writes distances directly into fr.grid. Returns the number of
// evaluations performed.
func (fr *FlatRenderer) evalKRange(kStart, kEnd int, posbuf []ms3.Vec, distbuf []float32, userData any) (uint64, error) {
	bufSize := len(posbuf)
	sz := (fr.nx + 1) * (fr.ny + 1) // points per k-plane
	gridOffset := kStart * sz
	batchStart := gridOffset
	posIdx := 0
	var evals uint64
	for k := kStart; k < kEnd; k++ {
		for j := 0; j <= fr.ny; j++ {
			for i := 0; i <= fr.nx; i++ {
				posbuf[posIdx] = ms3.Vec{
					X: fr.origin.X + float32(i)*fr.res,
					Y: fr.origin.Y + float32(j)*fr.res,
					Z: fr.origin.Z + float32(k)*fr.res,
				}
				posIdx++
				if posIdx == bufSize {
					if err := fr.s.Evaluate(posbuf, distbuf, userData); err != nil {
						return evals, err
					}
					copy(fr.grid[batchStart:], distbuf)
					evals += uint64(bufSize)
					batchStart += bufSize
					posIdx = 0
				}
			}
		}
	}
	if posIdx > 0 {
		if err := fr.s.Evaluate(posbuf[:posIdx], distbuf[:posIdx], userData); err != nil {
			return evals, err
		}
		copy(fr.grid[batchStart:], distbuf[:posIdx])
		evals += uint64(posIdx)
	}
	return evals, nil
}

// ReadTriangles implements [Renderer]. On the first call it evaluates all grid
// corners; subsequent calls iterate over cubes and emit marching-cubes triangles.
func (fr *FlatRenderer) ReadTriangles(dst []ms3.Triangle, userData any) (n int, err error) {
	if len(dst) < marchingCubesMaxTriangles {
		return 0, io.ErrShortBuffer
	}
	if !fr.initialized {
		if err = fr.evalGrid(userData); err != nil {
			return 0, err
		}
		fr.initialized = true
		fr.posbuf = nil // free evaluation buffers
		fr.distbuf = nil
	}

	sy := fr.nx + 1
	sz := sy * (fr.ny + 1)
	totalCubes := fr.nx * fr.ny * fr.nz
	cubeDiag := 2 * sqrt3 * fr.res

	for len(dst)-n >= marchingCubesMaxTriangles {
		if fr.cubeIdx >= totalCubes {
			return n, io.EOF
		}
		ci := fr.cubeIdx
		fr.cubeIdx++
		cx := ci % fr.nx
		cy := (ci / fr.nx) % fr.ny
		cz := ci / (fr.nx * fr.ny)

		// Grid index of corner (cx, cy, cz) — the (0,0,0) corner of this cube.
		base := cx + cy*sy + cz*sz

		// Quick reject using corner 0: if it is far from the surface, skip.
		if math32.Abs(fr.grid[base]) > cubeDiag {
			continue
		}

		// Corner distances matching CubeCorners ordering:
		// 0:(0,0,0) 1:(+x,0,0) 2:(+x,+y,0) 3:(0,+y,0)
		// 4:(0,0,+z) 5:(+x,0,+z) 6:(+x,+y,+z) 7:(0,+y,+z)
		var v [8]float32
		v[0] = fr.grid[base]
		v[1] = fr.grid[base+1]
		v[2] = fr.grid[base+1+sy]
		v[3] = fr.grid[base+sy]
		v[4] = fr.grid[base+sz]
		v[5] = fr.grid[base+1+sz]
		v[6] = fr.grid[base+1+sy+sz]
		v[7] = fr.grid[base+sy+sz]

		ox := fr.origin.X + float32(cx)*fr.res
		oy := fr.origin.Y + float32(cy)*fr.res
		oz := fr.origin.Z + float32(cz)*fr.res
		r := fr.res
		var p [8]ms3.Vec
		p[0] = ms3.Vec{X: ox, Y: oy, Z: oz}
		p[1] = ms3.Vec{X: ox + r, Y: oy, Z: oz}
		p[2] = ms3.Vec{X: ox + r, Y: oy + r, Z: oz}
		p[3] = ms3.Vec{X: ox, Y: oy + r, Z: oz}
		p[4] = ms3.Vec{X: ox, Y: oy, Z: oz + r}
		p[5] = ms3.Vec{X: ox + r, Y: oy, Z: oz + r}
		p[6] = ms3.Vec{X: ox + r, Y: oy + r, Z: oz + r}
		p[7] = ms3.Vec{X: ox, Y: oy + r, Z: oz + r}

		n += mcToTriangles(dst[n:], p, v, 0)
	}

	if fr.cubeIdx >= totalCubes {
		return n, io.EOF
	}
	return n, nil
}
