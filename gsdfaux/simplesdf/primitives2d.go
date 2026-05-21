package simplesdf

import "github.com/soypat/geometry/ms2"

// Circle creates a circle of radius r centered at the origin.
func Circle(r float64) SDF2 { return wrap2("Circle", bld.NewCircle(float32(r))) }

// Rect creates a rectangle with the given x and y dimensions centered at the origin.
func Rect(x, y float64) SDF2 { return wrap2("Rect", bld.NewRectangle(float32(x), float32(y))) }

// Hexagon creates a regular hexagon with the given side length.
func Hexagon(side float64) SDF2 { return wrap2("Hexagon", bld.NewHexagon(float32(side))) }

// Ellipse creates an ellipse with semi-axes a (x) and b (y).
func Ellipse(a, b float64) SDF2 { return wrap2("Ellipse", bld.NewEllipse(float32(a), float32(b))) }

// Arc creates a circular arc with the given radius, arc angle (radians), and stroke thickness.
func Arc(radius, arcAngle, thickness float64) SDF2 {
	return wrap2("Arc", bld.NewArc(float32(radius), float32(arcAngle), float32(thickness)))
}

// Polygon creates a filled polygon from the given vertices expressed as [x, y] pairs.
func Polygon(points [][2]float64) SDF2 {
	vecs := make([]ms2.Vec, len(points))
	for i, p := range points {
		vecs[i] = ms2.Vec{X: float32(p[0]), Y: float32(p[1])}
	}
	return wrap2("Polygon", bld.NewPolygon(vecs))
}
