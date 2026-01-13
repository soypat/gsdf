package glrender

import (
	"os"
	"testing"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/gleval"
)

// TestQEFSolver tests the QEF (Quadratic Error Function) solver used for vertex placement.
// The QEF finds a point x that minimizes the sum of squared distances to tangent planes:
//
//	minimize sum_i (n_i · (x - p_i))^2
//
// For a sphere centered at origin, the optimal vertex for a voxel at the surface
// should lie close to the surface.
func TestQEFSolver(t *testing.T) {
	// Test case: 3 orthogonal planes intersecting at a known point.
	// If we have:
	//   - plane 1: normal (1,0,0), point (0.5, 0, 0) -> n·(x-p) = x - 0.5 = 0
	//   - plane 2: normal (0,1,0), point (0, 0.5, 0) -> n·(x-p) = y - 0.5 = 0
	//   - plane 3: normal (0,0,1), point (0, 0, 0.5) -> n·(x-p) = z - 0.5 = 0
	// The unique solution is x = (0.5, 0.5, 0.5)

	points := []ms3.Vec{
		{X: 0.5, Y: 0, Z: 0},
		{X: 0, Y: 0.5, Z: 0},
		{X: 0, Y: 0, Z: 0.5},
	}
	normals := []ms3.Vec{
		{X: 1, Y: 0, Z: 0},
		{X: 0, Y: 1, Z: 0},
		{X: 0, Y: 0, Z: 1},
	}

	// Solve QEF using the same method as DualContourLeastSquares
	cubeOrigin := ms3.Vec{X: 0, Y: 0, Z: 0}
	var AtA ms3.Mat3
	var Atb ms3.Vec

	for i := 0; i < len(points); i++ {
		pi := points[i]
		qi := ms3.Sub(pi, cubeOrigin)
		ni := ms3.Unit(normals[i])

		// Outer product n * n^T
		outer := ms3.Prod(ni, ni)
		AtA = ms3.AddMat3(AtA, outer)

		// n * (n · q)
		dot := ms3.Dot(ni, qi)
		scaledNi := ms3.Scale(dot, ni)
		Atb = ms3.Add(Atb, scaledNi)
	}

	// Add small regularization for numerical stability
	lambda := float32(1e-6)
	bias := vertMean(points)
	AtA = ms3.AddMat3(AtA, ms3.ScaleMat3(ms3.IdentityMat3(), lambda))
	Atb = ms3.Add(Atb, ms3.Scale(lambda, ms3.Sub(bias, cubeOrigin)))

	det := AtA.Determinant()
	if math32.Abs(det) < 1e-10 {
		t.Fatal("matrix is singular")
	}

	AtAInv := AtA.Inverse()
	x := ms3.MulMatVec(AtAInv, Atb)
	result := ms3.Add(x, cubeOrigin)

	expected := ms3.Vec{X: 0.5, Y: 0.5, Z: 0.5}
	tol := float32(1e-4)
	if !vecNear(result, expected, tol) {
		t.Errorf("QEF solution: got %v, want %v (within %v)", result, expected, tol)
	}
}

// TestQEFSolverDiagonalPlanes tests QEF with non-axis-aligned normals.
func TestQEFSolverDiagonalPlanes(t *testing.T) {
	// Three planes meeting at (1, 1, 1):
	// Each plane passes through (1,1,1) with different normals
	target := ms3.Vec{X: 1, Y: 1, Z: 1}

	// Create 3 non-degenerate normals
	normals := []ms3.Vec{
		ms3.Unit(ms3.Vec{X: 1, Y: 1, Z: 0}),
		ms3.Unit(ms3.Vec{X: 0, Y: 1, Z: 1}),
		ms3.Unit(ms3.Vec{X: 1, Y: 0, Z: 1}),
	}

	// Points on each plane (all passing through target)
	points := make([]ms3.Vec, len(normals))
	for i := range normals {
		points[i] = target // All planes pass through target
	}

	cubeOrigin := ms3.Vec{X: 0.5, Y: 0.5, Z: 0.5}
	var AtA ms3.Mat3
	var Atb ms3.Vec

	for i := 0; i < len(points); i++ {
		pi := points[i]
		qi := ms3.Sub(pi, cubeOrigin)
		ni := normals[i] // Already unit

		outer := ms3.Prod(ni, ni)
		AtA = ms3.AddMat3(AtA, outer)

		dot := ms3.Dot(ni, qi)
		scaledNi := ms3.Scale(dot, ni)
		Atb = ms3.Add(Atb, scaledNi)
	}

	// Minimal regularization
	lambda := float32(1e-8)
	bias := vertMean(points)
	AtA = ms3.AddMat3(AtA, ms3.ScaleMat3(ms3.IdentityMat3(), lambda))
	Atb = ms3.Add(Atb, ms3.Scale(lambda, ms3.Sub(bias, cubeOrigin)))

	det := AtA.Determinant()
	if math32.Abs(det) < 1e-10 {
		t.Fatalf("matrix is singular, det=%v", det)
	}

	AtAInv := AtA.Inverse()
	x := ms3.MulMatVec(AtAInv, Atb)
	result := ms3.Add(x, cubeOrigin)

	tol := float32(1e-3)
	if !vecNear(result, target, tol) {
		t.Errorf("QEF diagonal planes: got %v, want %v (within %v)", result, target, tol)
	}
}

// TestDualContourSphereVerticesOnSurface tests that dual contour vertices
// for a sphere lie close to the actual sphere surface.
func TestDualContourSphereVerticesOnSurface(t *testing.T) {
	const (
		radius = 1.0
		res    = radius / 8 // 8 divisions across diameter
	)

	var bld gsdf.Builder
	shape := bld.NewSphere(radius)
	sdf, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		t.Fatal(err)
	}

	var dcr DualContourRenderer
	var vp gleval.VecPool
	err = dcr.Reset(sdf, res, &DualContourLeastSquares{}, &vp)
	if err != nil {
		t.Fatal(err)
	}

	tris, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}

	if len(tris) == 0 {
		t.Fatal("no triangles generated")
	}

	// Collect unique vertices
	vertSet := make(map[ms3.Vec]struct{})
	for _, tri := range tris {
		vertSet[tri[0]] = struct{}{}
		vertSet[tri[1]] = struct{}{}
		vertSet[tri[2]] = struct{}{}
	}

	// Check each vertex is close to the sphere surface
	var maxDist float32
	var maxDistVert ms3.Vec
	var totalDist float32
	surfaceTol := float32(res * 1.5) // Vertices should be within ~1.5 voxels of surface (curved surfaces may exceed 1x)

	verts := make([]ms3.Vec, 0, len(vertSet))
	for v := range vertSet {
		verts = append(verts, v)
	}

	dists := make([]float32, len(verts))
	err = sdf.Evaluate(verts, dists, &vp)
	if err != nil {
		t.Fatal(err)
	}

	for i, v := range verts {
		dist := math32.Abs(dists[i])
		totalDist += dist
		if dist > maxDist {
			maxDist = dist
			maxDistVert = v
		}
	}

	avgDist := totalDist / float32(len(verts))

	t.Logf("Sphere dual contour stats:")
	t.Logf("  Triangles: %d", len(tris))
	t.Logf("  Unique vertices: %d", len(verts))
	t.Logf("  Max distance from surface: %.6f (vertex: %v)", maxDist, maxDistVert)
	t.Logf("  Avg distance from surface: %.6f", avgDist)
	t.Logf("  Resolution: %.6f", res)

	if maxDist > surfaceTol {
		t.Errorf("vertex too far from surface: max dist %.6f > tolerance %.6f", maxDist, surfaceTol)
	}

	// Average distance should be much smaller than max tolerance
	if avgDist > surfaceTol/4 {
		t.Errorf("average distance too high: %.6f > %.6f", avgDist, surfaceTol/4)
	}
}

// TestDualContourBoxVerticesOnSurface tests vertex placement for a box shape.
// Boxes are interesting because they have sharp features (edges, corners).
func TestDualContourBoxVerticesOnSurface(t *testing.T) {
	const (
		boxSize = 2.0
		res     = boxSize / 8
	)

	var bld gsdf.Builder
	shape := bld.NewBox(boxSize, boxSize, boxSize, 0)
	sdf, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		t.Fatal(err)
	}

	var dcr DualContourRenderer
	var vp gleval.VecPool
	err = dcr.Reset(sdf, res, &DualContourLeastSquares{}, &vp)
	if err != nil {
		t.Fatal(err)
	}

	tris, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}

	if len(tris) == 0 {
		t.Fatal("no triangles generated")
	}

	// Collect unique vertices
	vertSet := make(map[ms3.Vec]struct{})
	for _, tri := range tris {
		vertSet[tri[0]] = struct{}{}
		vertSet[tri[1]] = struct{}{}
		vertSet[tri[2]] = struct{}{}
	}

	verts := make([]ms3.Vec, 0, len(vertSet))
	for v := range vertSet {
		verts = append(verts, v)
	}

	dists := make([]float32, len(verts))
	err = sdf.Evaluate(verts, dists, &vp)
	if err != nil {
		t.Fatal(err)
	}

	var maxDist float32
	var totalDist float32
	surfaceTol := float32(res * 1.5) // Box corners may be slightly further

	for _, dist := range dists {
		absDist := math32.Abs(dist)
		totalDist += absDist
		if absDist > maxDist {
			maxDist = absDist
		}
	}

	avgDist := totalDist / float32(len(verts))

	t.Logf("Box dual contour stats:")
	t.Logf("  Triangles: %d", len(tris))
	t.Logf("  Unique vertices: %d", len(verts))
	t.Logf("  Max distance from surface: %.6f", maxDist)
	t.Logf("  Avg distance from surface: %.6f", avgDist)

	if maxDist > surfaceTol {
		t.Errorf("vertex too far from surface: max dist %.6f > tolerance %.6f", maxDist, surfaceTol)
	}
}

// TestDualContourCompareWithNaive compares least squares vertex placement
// against the naive "mean of intersections" approach.
// The QEF solution should generally be closer to the surface.
func TestDualContourCompareWithNaive(t *testing.T) {
	const (
		radius = 1.0
		res    = radius / 6
	)

	var bld gsdf.Builder
	shape := bld.NewSphere(radius)
	sdf, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		t.Fatal(err)
	}

	// Test with least squares
	var dcr DualContourRenderer
	var vp gleval.VecPool
	err = dcr.Reset(sdf, res, &DualContourLeastSquares{}, &vp)
	if err != nil {
		t.Fatal(err)
	}

	trisLSQ, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}

	// Test with naive (mean) placement
	err = dcr.Reset(sdf, res, &DualContourNaive{}, &vp)
	if err != nil {
		t.Fatal(err)
	}

	trisNaive, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}

	// Compute average surface distances
	avgDistLSQ := computeAvgSurfaceDist(t, trisLSQ, sdf, &vp)
	avgDistNaive := computeAvgSurfaceDist(t, trisNaive, sdf, &vp)

	t.Logf("Comparison (sphere r=%v, res=%v):", radius, res)
	t.Logf("  Least squares: %d triangles, avg dist %.6f", len(trisLSQ), avgDistLSQ)
	t.Logf("  Naive mean:    %d triangles, avg dist %.6f", len(trisNaive), avgDistNaive)

	// QEF should produce vertices at least as close to surface as naive
	// (with some tolerance for numerical issues)
	tolerance := float32(0.01)
	if avgDistLSQ > avgDistNaive+tolerance {
		t.Errorf("least squares produced worse results than naive: %.6f > %.6f", avgDistLSQ, avgDistNaive)
	}
}

// DualContourNaive is a simple vertex placement that uses the mean of edge intersections.
type DualContourNaive struct{}

func (n *DualContourNaive) PlaceVertices(cubes []DualCube, origin ms3.Vec, res float32, sdf gleval.SDF3, posbuf []ms3.Vec, distbuf []float32, userData any) error {
	for c := range cubes {
		cube := &cubes[c]
		if len(cube.Neighbors) == 0 {
			continue
		}

		// Collect edge intersection points
		var sum ms3.Vec
		count := 0
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
			sum = ms3.Add(sum, contrib)
			count++
		}

		if count > 0 {
			cubes[c].FinalVertex = ms3.Scale(1.0/float32(count), sum)
		}
	}
	return nil
}

func computeAvgSurfaceDist(t *testing.T, tris []ms3.Triangle, sdf gleval.SDF3, userData any) float32 {
	t.Helper()

	vertSet := make(map[ms3.Vec]struct{})
	for _, tri := range tris {
		vertSet[tri[0]] = struct{}{}
		vertSet[tri[1]] = struct{}{}
		vertSet[tri[2]] = struct{}{}
	}

	verts := make([]ms3.Vec, 0, len(vertSet))
	for v := range vertSet {
		verts = append(verts, v)
	}

	if len(verts) == 0 {
		return 0
	}

	dists := make([]float32, len(verts))
	err := sdf.Evaluate(verts, dists, userData)
	if err != nil {
		t.Fatal(err)
	}

	var total float32
	for _, d := range dists {
		total += math32.Abs(d)
	}
	return total / float32(len(verts))
}

func vecNear(a, b ms3.Vec, tol float32) bool {
	return math32.Abs(a.X-b.X) < tol &&
		math32.Abs(a.Y-b.Y) < tol &&
		math32.Abs(a.Z-b.Z) < tol
}

// TestDualContourMissingNeighbors checks if there are missing neighbors in quad generation.
func TestDualContourMissingNeighbors(t *testing.T) {
	const (
		radius = 1.0
		res    = radius / 8
	)

	var bld gsdf.Builder
	shape := bld.NewSphere(radius)
	sdf, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		t.Fatal(err)
	}

	var dcr DualContourRenderer
	var vp gleval.VecPool
	err = dcr.Reset(sdf, res, &DualContourLeastSquares{}, &vp)
	if err != nil {
		t.Fatal(err)
	}

	tris, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}

	// Check for suspicious vertices (very close to origin or outside bounds)
	bb := sdf.Bounds()
	var suspiciousVerts []ms3.Vec
	for _, tri := range tris {
		for _, v := range tri {
			// Check if vertex is at/near origin (inside the sphere)
			distFromOrigin := math32.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
			if distFromOrigin < radius*0.5 {
				suspiciousVerts = append(suspiciousVerts, v)
			}
			// Check if vertex is way outside bounds
			if v.X < bb.Min.X-res || v.X > bb.Max.X+res ||
				v.Y < bb.Min.Y-res || v.Y > bb.Max.Y+res ||
				v.Z < bb.Min.Z-res || v.Z > bb.Max.Z+res {
				suspiciousVerts = append(suspiciousVerts, v)
			}
		}
	}

	if len(suspiciousVerts) > 0 {
		// Deduplicate
		seen := make(map[ms3.Vec]struct{})
		unique := []ms3.Vec{}
		for _, v := range suspiciousVerts {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				unique = append(unique, v)
			}
		}
		t.Errorf("Found %d suspicious vertices (inside sphere or outside bounds):", len(unique))
		for i, v := range unique {
			if i >= 10 {
				t.Errorf("  ... and %d more", len(unique)-10)
				break
			}
			dist := math32.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
			t.Errorf("  %v (dist from origin: %.4f)", v, dist)
		}
	}
}

// TestBenchmarkSnowman creates a snowman shape (union of 3 spheres)
// matching Python sdftoolbox's hello_dualiso.py example.
// Python uses: Grid(res=(65,65,65), min_corner=(-1.5,-1.5,-1.5), max_corner=(1.5,1.5,1.5))
// which gives spacing = 3.0 / 64 = 0.046875
func TestBenchmarkSnowman(t *testing.T) {
	const res = 3.0 / 64 // Match Python: 65 samples over [-1.5, 1.5] = 64 intervals

	var bld gsdf.Builder
	// Create snowman: 3 spheres stacked vertically (matching Python dimensions)
	body := bld.NewSphere(0.4)
	middle := bld.Translate(bld.NewSphere(0.3), 0, 0, 0.45)
	head := bld.Translate(bld.NewSphere(0.2), 0, 0, 0.8)
	snowman := bld.Union(body, middle, head)

	sdf, err := gleval.NewCPUSDF3(snowman)
	if err != nil {
		t.Fatal(err)
	}

	var dcr DualContourRenderer
	var vp gleval.VecPool
	err = dcr.Reset(sdf, res, &DualContourLeastSquares{}, &vp)
	if err != nil {
		t.Fatal(err)
	}

	tris, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Snowman: %d triangles, res=%.6f", len(tris), res)

	fp, _ := os.Create("benchmark_snowman.stl")
	WriteBinarySTL(fp, tris)
	fp.Close()
}

// TestBenchmarkBox creates a box to test sharp edge preservation.
func TestBenchmarkBox(t *testing.T) {
	const res = 3.0 / 64 // Match Python resolution

	var bld gsdf.Builder
	box := bld.NewBox(1, 2, 0.5, 0) // No rounding

	sdf, err := gleval.NewCPUSDF3(box)
	if err != nil {
		t.Fatal(err)
	}

	var dcr DualContourRenderer
	var vp gleval.VecPool
	err = dcr.Reset(sdf, res, &DualContourLeastSquares{}, &vp)
	if err != nil {
		t.Fatal(err)
	}

	tris, err := dcr.RenderAll(nil, &vp)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Box: %d triangles, res=%.6f", len(tris), res)

	fp, _ := os.Create("benchmark_box.stl")
	WriteBinarySTL(fp, tris)
	fp.Close()
}
