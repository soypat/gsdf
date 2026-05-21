package simplesdf

// Sphere creates a sphere of radius r centered at the origin.
func Sphere(r float64) SDF3 { return wrap3("Sphere", bld.NewSphere(float32(r))) }

// Box creates a box with the given dimensions and optional edge rounding radius.
func Box(x, y, z, round float64) SDF3 {
	return wrap3("Box", bld.NewBox(float32(x), float32(y), float32(z), float32(round)))
}

// Cylinder creates a cylinder aligned along Z with radius r, height h, and optional edge rounding.
func Cylinder(r, h, round float64) SDF3 {
	return wrap3("Cylinder", bld.NewCylinder(float32(r), float32(h), float32(round)))
}

// Torus creates a torus with the given major (ring) and minor (tube) radii.
func Torus(major, minor float64) SDF3 {
	return wrap3("Torus", bld.NewTorus(float32(major), float32(minor)))
}

// HexPrism creates a hexagonal prism. face2face is the distance between opposite flat faces; h is the height.
func HexPrism(face2face, h float64) SDF3 {
	return wrap3("HexPrism", bld.NewHexagonalPrism(float32(face2face), float32(h)))
}

// TriPrism creates a triangular prism with the given triangle bisector height and extrusion length.
func TriPrism(triHeight, extrudeLen float64) SDF3 {
	return wrap3("TriPrism", bld.NewTriangularPrism(float32(triHeight), float32(extrudeLen)))
}

// BoxFrame creates a wireframe box outline with the given dimensions and edge thickness.
func BoxFrame(x, y, z, edgeThickness float64) SDF3 {
	return wrap3("BoxFrame", bld.NewBoxFrame(float32(x), float32(y), float32(z), float32(edgeThickness)))
}
