package gsdf

import (
	"errors"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/glbuild"
)

// NewBoundsBoxFrame creates a BoxFrame from a bb ([ms3.Box]) such that the BoxFrame envelops the bb.
// Useful for debugging bounding boxes of [glbuild.Shader3D] primitives and operations.
func NewBoundsBoxFrame(bb ms3.Box) (glbuild.Shader3D, error) {
	size := bb.Size()
	frameThickness := size.Max() / 256
	// Bounding box's frames protrude.
	size = ms3.AddScalar(2*frameThickness, size)
	bounding, err := NewBoxFrame(size.X, size.Y, size.Z, frameThickness)
	if err != nil {
		return nil, err
	}
	center := bb.Center()
	bounding = Translate(bounding, center.X, center.Y, center.Z)
	return bounding, nil
}

type sphere struct {
	r float32
}

func NewSphere(r float32) (glbuild.Shader3D, error) {
	valid := r > 0
	if !valid {
		return nil, errors.New("zero or negative sphere radius")
	}
	return &sphere{r: r}, nil
}

func (s *sphere) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (s *sphere) AppendShaderName(b []byte) []byte {
	b = append(b, "sphere"...)
	b = fappend(b, s.r, 'n', 'p')
	return b
}

func (s *sphere) AppendShaderBody(b []byte) []byte {
	b = append(b, "return length(p)-"...)
	b = fappend(b, s.r, '-', '.')
	b = append(b, ';')
	return b
}

func (s *sphere) Bounds() ms3.Box {
	return ms3.Box{
		Min: ms3.Vec{X: -s.r, Y: -s.r, Z: -s.r},
		Max: ms3.Vec{X: s.r, Y: s.r, Z: s.r},
	}
}

func NewBox(x, y, z, round float32) (glbuild.Shader3D, error) {
	if round < 0 || round > x/2 || round > y/2 || round > z/2 {
		return nil, errors.New("invalid box rounding value")
	} else if x <= 0 || y <= 0 || z <= 0 {
		return nil, errors.New("zero or negative box dimension")
	}
	return &box{dims: ms3.Vec{X: x, Y: y, Z: z}, round: round}, nil
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
	b = vecappend(b, s.dims, 'i', 'n', 'p')
	b = fappend(b, s.round, 'n', 'p')
	return b
}

func (s *box) AppendShaderBody(b []byte) []byte {
	b = appendFloatDecl(b, "r", s.round)
	b = appendVec3Decl(b, "d", ms3.Scale(0.5, s.dims)) // Inigo's SDF is x2 size.
	b = append(b, `vec3 q = abs(p)-d+r;
return length(max(q,0.0)) + min(max(q.x,max(q.y,q.z)),0.0)-r;`...)
	return b
}

func (s *box) Bounds() ms3.Box {
	return ms3.NewCenteredBox(ms3.Vec{}, s.dims)
}

func NewCylinder(r, h, rounding float32) (glbuild.Shader3D, error) {
	if rounding < 0 || rounding >= r || rounding > h/2 {
		return nil, errors.New("invalid cylinder rounding")
	}
	if r <= 0 || h <= 0 {
		return nil, errors.New("zero or negative cylinder dimension")
	}
	return &cylinder{r: r, h: h, round: rounding}, nil
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
	b = fappend(b, s.r, 'n', 'p')
	b = fappend(b, s.h, 'n', 'p')
	b = fappend(b, s.round, 'n', 'p')
	return b
}

func (s *cylinder) AppendShaderBody(b []byte) []byte {
	r, h, round := s.args()
	b = append(b, "p = p.xzy;\n"...)
	b = appendFloatDecl(b, "r", r)
	b = appendFloatDecl(b, "h", h) // Correct height for rounding effect.
	if s.round == 0 {
		b = append(b, `vec2 d = abs(vec2(length(p.xz),p.y)) - vec2(r,h);
return min(max(d.x,d.y),0.0) + length(max(d,0.0));`...)
	} else {
		b = appendFloatDecl(b, "rd", round)
		b = append(b, `vec2 d = vec2( length(p.xz)-r+rd, abs(p.y) - h );
return min(max(d.x,d.y),0.0) + length(max(d,0.0)) - rd;`...)
	}
	return b
}

func (c *cylinder) args() (r, h, round float32) {
	return c.r, (c.h - 2*c.round) / 2, c.round
}

func NewHexagonalPrism(face2Face, h float32) (glbuild.Shader3D, error) {
	if face2Face <= 0 || h <= 0 {
		return nil, errors.New("invalid hexagonal prism parameter")
	}
	return &hex{side: face2Face, h: h}, nil
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
	b = fappend(b, s.side, 'n', 'p')
	b = fappend(b, s.h, 'n', 'p')
	return b
}

func (s *hex) AppendShaderBody(b []byte) []byte {
	b = appendFloatDecl(b, "_h", s.h)
	b = appendFloatDecl(b, "side", s.side)
	b = append(b, `vec2 h = vec2(side, _h);
const vec3 k = vec3(-0.8660254038, 0.5, 0.57735);
p = abs(p);
p.xy -= 2.0*min(dot(k.xy, p.xy), 0.0)*k.xy;
vec2 aux = p.xy-vec2(clamp(p.x,-k.z*h.x,k.z*h.x), h.x);
vec2 d = vec2( length(aux)*sign(p.y-h.x), p.z-h.y );
return min(max(d.x,d.y),0.0) + length(max(d,0.0));`...)
	return b
}

func NewTriangularPrism(triHeight, extrudeLength float32) (glbuild.Shader3D, error) {
	if triHeight <= 0 || extrudeLength <= 0 {
		return nil, errors.New("invalid triangular prism parameter")
	}
	return &tri{height: triHeight, extrudeLength: extrudeLength}, nil
}

type tri struct {
	height        float32
	extrudeLength float32
}

func (t *tri) args() (h1, h2 float32) {
	return t.height / 3, t.extrudeLength / 2
}

func (t *tri) Bounds() ms3.Box {
	height := t.height
	side := height / tribisect
	longBisect := side / sqrt3    // (L/2)/cosd(30)
	shortBisect := longBisect / 2 // (L/2)/tand(60)
	hd2 := t.extrudeLength / 2
	return ms3.Box{
		Min: ms3.Vec{X: -side / 2, Y: -shortBisect, Z: -hd2},
		Max: ms3.Vec{X: side / 2, Y: longBisect, Z: hd2},
	}
}

func (t *tri) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (t *tri) AppendShaderName(b []byte) []byte {
	b = append(b, "tri"...)
	b = fappend(b, t.height, 'n', 'p')
	b = fappend(b, t.extrudeLength, 'n', 'p')
	return b
}

func (t *tri) AppendShaderBody(b []byte) []byte {
	h1, h2 := t.args()
	b = appendVec2Decl(b, "h", ms2.Vec{X: h1, Y: h2})
	b = append(b, `vec3 q = abs(p);
return max(q.z-h.y,max(q.x*0.8660254038+p.y*0.5,-p.y)-h.x);`...)
	return b
}

type torus struct {
	rRing, rGreater float32
}

func NewTorus(greaterRadius, ringRadius float32) (glbuild.Shader3D, error) {
	if greaterRadius < 2*ringRadius {
		return nil, errors.New("too large torus ring radius")
	} else if greaterRadius <= 0 || ringRadius <= 0 {
		return nil, errors.New("invalid torus parameter")
	}
	return &torus{rRing: ringRadius, rGreater: greaterRadius}, nil
}

func (s *torus) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}

func (s *torus) AppendShaderName(b []byte) []byte {
	b = append(b, "torus"...)
	b = fappend(b, s.rRing, 'n', 'p')
	b = fappend(b, s.rGreater, 'n', 'p')
	return b
}

func (s *torus) AppendShaderBody(b []byte) []byte {
	b = appendFloatDecl(b, "t1", s.rGreater) // Counteract rounding effect.
	b = appendFloatDecl(b, "t2", s.rRing)
	b = append(b, `p = p.xzy;
vec2 t = vec2(t1, t2);
vec2 q = vec2(length(p.xz)-t.x,p.y);
return length(q)-t.y;`...)
	return b
}

func (s *torus) Bounds() ms3.Box {
	R := s.rRing + s.rGreater
	return ms3.Box{
		Min: ms3.Vec{X: -R, Y: -R, Z: -s.rRing},
		Max: ms3.Vec{X: R, Y: R, Z: s.rRing},
	}
}

func NewBoxFrame(dimX, dimY, dimZ, e float32) (glbuild.Shader3D, error) {
	e /= 2
	if dimX <= 0 || dimY <= 0 || dimZ <= 0 || e <= 0 {
		return nil, errors.New("negative or zero BoxFrame dimension")
	}
	d := ms3.Vec{X: dimX, Y: dimY, Z: dimZ}
	if 2*e > d.Min() {
		return nil, errors.New("BoxFrame edge thickness too large")
	}
	return &boxframe{dims: d, e: e}, nil
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
	b = vecappend(b, bf.dims, 'i', 'n', 'p')
	b = fappend(b, bf.e, 'n', 'p')
	return b
}

func (bf *boxframe) AppendShaderBody(b []byte) []byte {
	e, bb := bf.args()
	b = appendFloatDecl(b, "e", e)
	b = appendVec3Decl(b, "b", bb)
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
