package gsdf

import (
	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/glbuild"
)

// NewBoundsBoxFrame creates a BoxFrame from a bb ([ms3.Box]) such that the BoxFrame envelops the bb.
// Useful for debugging bounding boxes of [glbuild.Shader3D] primitives and operations.
func (bld *Builder) NewBoundsBoxFrame(bb ms3.Box) glbuild.Shader3D {
	size := bb.Size()
	frameThickness := size.Max() / 256
	// Bounding box's frames protrude.
	size = ms3.AddScalar(2*frameThickness, size)
	bounding := bld.NewBoxFrame(size.X, size.Y, size.Z, frameThickness)
	center := bb.Center()
	bounding = bld.Translate(bounding, center.X, center.Y, center.Z)
	return bounding
}

type sphere struct {
	r float32
}

// NewSphere creates a sphere centered at the origin of radius r.
func (bld *Builder) NewSphere(r float32) glbuild.Shader3D {
	valid := r > 0
	if !valid {
		bld.shapeErrorf("zero or negative sphere radius")
	}
	return &sphere{r: r}
}

func (s *sphere) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (s *sphere) AppendShaderName(b []byte) []byte {
	b = append(b, "sphere"...)
	b = glbuild.AppendFloat(b, 'n', 'p', s.r)
	return b
}

func (s *sphere) AppendShaderBody(b []byte) []byte {
	b = append(b, "return length(p)-"...)
	b = glbuild.AppendFloat(b, '-', '.', s.r)
	b = append(b, ';')
	return b
}

func (u *sphere) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

func (s *sphere) Bounds() ms3.Box {
	return ms3.Box{
		Min: ms3.Vec{X: -s.r, Y: -s.r, Z: -s.r},
		Max: ms3.Vec{X: s.r, Y: s.r, Z: s.r},
	}
}

// NewBox creates a box centered at the origin with x,y,z dimensions and a rounding parameter to round edges.
func (bld *Builder) NewBox(x, y, z, round float32) glbuild.Shader3D {
	if round < 0 || round > x/2 || round > y/2 || round > z/2 {
		bld.shapeErrorf("invalid box rounding value")
	}
	if x <= 0 || y <= 0 || z <= 0 {
		bld.shapeErrorf("zero or negative box dimension")
	}
	return &box{dims: ms3.Vec{X: x, Y: y, Z: z}, round: round}
}

type box struct {
	dims  ms3.Vec
	round float32
}

func (s *box) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (s *box) AppendShaderName(b []byte) []byte {
	b = append(b, "box"...)
	arr := s.dims.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	b = glbuild.AppendFloat(b, 'n', 'p', s.round)
	return b
}

func (s *box) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "r", s.round)
	b = glbuild.AppendVec3Decl(b, "d", ms3.Scale(0.5, s.dims)) // Inigo's SDF is x2 size.
	b = append(b, `vec3 q = abs(p)-d+r;
return length(max(q,0.0)) + min(max(q.x,max(q.y,q.z)),0.0)-r;`...)
	return b
}

func (u *box) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

func (s *box) Bounds() ms3.Box {
	return ms3.NewCenteredBox(ms3.Vec{}, s.dims)
}

// NewCylinder creates a cylinder centered at the origin with given radius and height.
// The cylinder's axis points in z direction.
func (bld *Builder) NewCylinder(r, h, rounding float32) glbuild.Shader3D {
	okRounding := rounding >= 0 && rounding < r && rounding < h/2
	if !okRounding {
		bld.shapeErrorf("invalid cylinder rounding")
	}
	okDim := r > 0 && h > 0
	if !okDim {
		bld.shapeErrorf("bad cylinder dimension")
	}
	return &cylinder{r: r, h: h, round: rounding}
}

type cylinder struct {
	r     float32
	h     float32
	round float32
}

func (s *cylinder) Bounds() ms3.Box {
	return ms3.Box{
		Min: ms3.Vec{X: -s.r, Y: -s.r, Z: -s.h / 2},
		Max: ms3.Vec{X: s.r, Y: s.r, Z: s.h / 2},
	}
}

func (s *cylinder) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (s *cylinder) AppendShaderName(b []byte) []byte {
	b = append(b, "cyl"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', s.r, s.h, s.round)
	return b
}

func (s *cylinder) AppendShaderBody(b []byte) []byte {
	r, h, round := s.args()
	b = append(b, "p = p.xzy;\n"...)
	b = glbuild.AppendFloatDecl(b, "r", r)
	b = glbuild.AppendFloatDecl(b, "h", h) // Correct height for rounding effect.
	if s.round == 0 {
		b = append(b, `vec2 d = abs(vec2(length(p.xz),p.y)) - vec2(r,h);
return min(max(d.x,d.y),0.0) + length(max(d,0.0));`...)
	} else {
		b = glbuild.AppendFloatDecl(b, "rd", round)
		b = append(b, `vec2 d = vec2( length(p.xz)-r+rd, abs(p.y) - h );
return min(max(d.x,d.y),0.0) + length(max(d,0.0)) - rd;`...)
	}
	return b
}

func (c *cylinder) args() (r, h, round float32) {
	return c.r, (c.h - 2*c.round) / 2, c.round
}

func (u *cylinder) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// NewHexagonalPrism creates a hexagonal prism given a face-to-face dimension and height.
// The hexagon's length is in the z axis.
func (bld *Builder) NewHexagonalPrism(face2Face, h float32) glbuild.Shader3D {
	if face2Face <= 0 || h <= 0 {
		bld.shapeErrorf("invalid hexagonal prism parameter")
	}
	return &hex{side: face2Face, h: h}
}

type hex struct {
	side float32
	h    float32
}

func (s *hex) Bounds() ms3.Box {
	l := s.side
	lx := l / tribisect
	return ms3.Box{
		Min: ms3.Vec{X: -lx, Y: -l, Z: -s.h},
		Max: ms3.Vec{X: lx, Y: l, Z: s.h},
	}
}

func (s *hex) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (s *hex) AppendShaderName(b []byte) []byte {
	b = append(b, "hex"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', s.side, s.h)
	return b
}

func (s *hex) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "_h", s.h)
	b = glbuild.AppendFloatDecl(b, "side", s.side)
	b = append(b, `vec2 h = vec2(side, _h);
const vec3 k = vec3(-0.8660254038, 0.5, 0.57735);
p = abs(p);
p.xy -= 2.0*min(dot(k.xy, p.xy), 0.0)*k.xy;
vec2 aux = p.xy-vec2(clamp(p.x,-k.z*h.x,k.z*h.x), h.x);
vec2 d = vec2( length(aux)*sign(p.y-h.x), p.z-h.y );
return min(max(d.x,d.y),0.0) + length(max(d,0.0));`...)
	return b
}

func (u *hex) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// NewTriangularPrism creates a 3D triangular prism with a given triangle cross-sectional height (2D)
// and a extrude length. The prism's extrude axis is in the z axis direction.
func (bld *Builder) NewTriangularPrism(triHeight, extrudeLength float32) glbuild.Shader3D {
	okExtrude := extrudeLength > 0 && !math32.IsInf(extrudeLength, 1)
	if !okExtrude {
		bld.shapeErrorf("bad triangular prism extrude length")
	}
	tri := bld.NewEquilateralTriangle(triHeight)
	return bld.Extrude(tri, extrudeLength)
}

type torus struct {
	rLesser, rGreater float32
}

// NewTorus creates a 3D torus given 2 radii to define the radius
// across (greaterRadius) and the "solid" radius (lesserRadius).
// If the radius were cut and stretched straight to form a cylinder the lesser
// radius would be the radius of the cylinder.
// The torus' axis is in the z axis.
func (bld *Builder) NewTorus(greaterRadius, lesserRadius float32) glbuild.Shader3D {
	if greaterRadius < 2*lesserRadius {
		bld.shapeErrorf("too large torus lesser radius")
	}
	if greaterRadius <= 0 || lesserRadius <= 0 {
		bld.shapeErrorf("invalid torus parameter")
	}
	return &torus{rLesser: lesserRadius, rGreater: greaterRadius}
}

func (s *torus) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (s *torus) AppendShaderName(b []byte) []byte {
	b = append(b, "torus"...)
	b = glbuild.AppendFloat(b, 'n', 'p', s.rLesser)
	b = glbuild.AppendFloat(b, 'n', 'p', s.rGreater)
	return b
}

func (s *torus) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "t1", s.rGreater) // Counteract rounding effect.
	b = glbuild.AppendFloatDecl(b, "t2", s.rLesser)
	b = append(b, `p = p.xzy;
vec2 t = vec2(t1, t2);
vec2 q = vec2(length(p.xz)-t.x,p.y);
return length(q)-t.y;`...)
	return b
}

func (s *torus) Bounds() ms3.Box {
	R := s.rLesser + s.rGreater
	return ms3.Box{
		Min: ms3.Vec{X: -R, Y: -R, Z: -s.rLesser},
		Max: ms3.Vec{X: R, Y: R, Z: s.rLesser},
	}
}

func (u *torus) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// NewBoxFrame creates a framed box with the frame being composed of square beams of thickness e.
func (bld *Builder) NewBoxFrame(dimX, dimY, dimZ, e float32) glbuild.Shader3D {
	e /= 2
	if dimX <= 0 || dimY <= 0 || dimZ <= 0 || e <= 0 {
		bld.shapeErrorf("negative or zero BoxFrame dimension")
	}
	d := ms3.Vec{X: dimX, Y: dimY, Z: dimZ}
	if 2*e > d.Min() {
		bld.shapeErrorf("BoxFrame edge thickness too large")
	}
	return &boxframe{dims: d, e: e}
}

type boxframe struct {
	dims ms3.Vec
	e    float32
}

func (bf *boxframe) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (bf *boxframe) AppendShaderName(b []byte) []byte {
	b = append(b, "boxframe"...)
	arr := bf.dims.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	b = glbuild.AppendFloat(b, 'n', 'p', bf.e)
	return b
}

func (bf *boxframe) AppendShaderBody(b []byte) []byte {
	e, bb := bf.args()
	b = glbuild.AppendFloatDecl(b, "e", e)
	b = glbuild.AppendVec3Decl(b, "b", bb)
	b = append(b, `p = abs(p)-b;
vec3 q = abs(p+e)-e;
return min(min(
      length(max(vec3(p.x,q.y,q.z),0.0))+min(max(p.x,max(q.y,q.z)),0.0),
      length(max(vec3(q.x,p.y,q.z),0.0))+min(max(q.x,max(p.y,q.z)),0.0)),
      length(max(vec3(q.x,q.y,p.z),0.0))+min(max(q.x,max(q.y,p.z)),0.0));`...)
	return b
}

func (bf *boxframe) Bounds() ms3.Box {
	return ms3.NewCenteredBox(ms3.Vec{}, bf.dims)
}

func (bf *boxframe) args() (e float32, b ms3.Vec) {
	dd, e := bf.dims, bf.e
	dd = ms3.Scale(0.5, dd)
	dd = ms3.AddScalar(-2*e, dd)
	return e, dd
}

func (u *boxframe) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}
