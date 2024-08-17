package gsdf

import (
	"errors"
	"fmt"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/glbuild"
)

// Union joins the shapes of two SDFs into one. Is exact.
func Union(s1, s2 glbuild.Shader3D) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		panic("nil object")
	}
	return &union{s1: s1, s2: s2}
}

type union struct {
	s1, s2 glbuild.Shader3D
}

func (u *union) Bounds() ms3.Box {
	return u.s1.Bounds().Union(u.s2.Bounds())
}

func (s *union) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *union) AppendShaderName(b []byte) []byte {
	b = append(b, "union_"...)
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *union) AppendShaderBody(b []byte) []byte {
	b = append(b, "return min("...)
	b = s.s1.AppendShaderName(b)
	b = append(b, "(p),"...)
	b = s.s2.AppendShaderName(b)
	b = append(b, "(p));"...)
	return b
}

// Difference is the SDF difference of a-b. Does not produce a true SDF.
func Difference(a, b glbuild.Shader3D) glbuild.Shader3D {
	if a == nil || b == nil {
		panic("nil argument to Difference")
	}
	return &diff{s1: a, s2: b}
}

type diff struct {
	s1, s2 glbuild.Shader3D // Performs s1-s2.
}

func (u *diff) Bounds() ms3.Box {
	return u.s1.Bounds()
}

func (s *diff) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *diff) AppendShaderName(b []byte) []byte {
	b = append(b, "diff_"...)
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *diff) AppendShaderBody(b []byte) []byte {
	b = appendDistanceDecl(b, s.s1, "a", "p")
	b = appendDistanceDecl(b, s.s2, "b", "p")
	b = append(b, "return max(a,-b);"...)
	return b
}

// Intersection is the SDF intersection of a ^ b. Does not produce an exact SDF.
func Intersection(a, b glbuild.Shader3D) glbuild.Shader3D {
	if a == nil || b == nil {
		panic("nil argument to Difference")
	}
	return &intersect{s1: a, s2: b}
}

type intersect struct {
	s1, s2 glbuild.Shader3D // Performs s1 ^ s2.
}

func (u *intersect) Bounds() ms3.Box {
	return u.s1.Bounds().Intersect(u.s2.Bounds())
}

func (s *intersect) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *intersect) AppendShaderName(b []byte) []byte {
	b = append(b, "intersect_"...)
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *intersect) AppendShaderBody(b []byte) []byte {
	b = append(b, "return max("...)
	b = s.s1.AppendShaderName(b)
	b = append(b, "(p),"...)
	b = s.s2.AppendShaderName(b)
	b = append(b, "(p));"...)
	return b
}

// Xor is the mutually exclusive boolean operation and results in an exact SDF.
func Xor(s1, s2 glbuild.Shader3D) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		panic("nil argument to Xor")
	}
	return &xor{s1: s1, s2: s2}
}

type xor struct {
	s1, s2 glbuild.Shader3D
}

func (u *xor) Bounds() ms3.Box {
	return u.s1.Bounds().Union(u.s2.Bounds())
}

func (s *xor) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *xor) AppendShaderName(b []byte) []byte {
	b = append(b, "xor_"...)
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *xor) AppendShaderBody(b []byte) []byte {
	b = appendDistanceDecl(b, s.s1, "d1", "(p)")
	b = appendDistanceDecl(b, s.s2, "d2", "(p)")
	b = append(b, "return max(min(d1,d2),-max(d1,d2));"...)
	return b
}

// Scale scales s by scaleFactor around the origin.
func Scale(s glbuild.Shader3D, scaleFactor float32) glbuild.Shader3D {
	return &scale{s: s, scale: scaleFactor}
}

type scale struct {
	s     glbuild.Shader3D
	scale float32
}

func (u *scale) Bounds() ms3.Box {
	b := u.s.Bounds()
	return b.Scale(ms3.Vec{X: u.scale, Y: u.scale, Z: u.scale})
}

func (s *scale) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *scale) AppendShaderName(b []byte) []byte {
	b = append(b, "scale_"...)
	b = s.s.AppendShaderName(b)
	return b
}

func (s *scale) AppendShaderBody(b []byte) []byte {
	b = appendFloatDecl(b, "s", s.scale)
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p/s)*s;"...)
	return b
}

// Symmetry reflects the SDF around one or more cartesian planes.
func Symmetry(s glbuild.Shader3D, mirrorX, mirrorY, mirrorZ bool) glbuild.Shader3D {
	if !mirrorX && !mirrorY && !mirrorZ {
		panic("ineffective symmetry")
	}

	return &symmetry{s: s, xyz: glbuild.NewXYZBits(mirrorX, mirrorY, mirrorZ)}
}

type symmetry struct {
	s   glbuild.Shader3D
	xyz glbuild.XYZBits
}

func (u *symmetry) Bounds() ms3.Box {
	box := u.s.Bounds()
	if u.xyz.X() {
		box.Min.X = minf(box.Min.X, -box.Max.X)
	}
	if u.xyz.Y() {
		box.Min.Y = minf(box.Min.Y, -box.Max.Y)
	}
	if u.xyz.Z() {
		box.Min.Z = minf(box.Min.Z, -box.Max.Z)
	}
	return box
}

func (s *symmetry) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *symmetry) AppendShaderName(b []byte) []byte {
	b = append(b, "symmetry"...)
	b = s.xyz.AppendMapped(b, [3]byte{'X', 'Y', 'Z'})
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *symmetry) AppendShaderBody(b []byte) []byte {
	b = append(b, "p."...)
	b = s.xyz.AppendMapped(b, [3]byte{'x', 'y', 'z'})
	b = append(b, "=abs(p."...)
	b = s.xyz.AppendMapped(b, [3]byte{'x', 'y', 'z'})
	b = append(b, ");\n return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p);"...)
	return b
}

// Transform applies a 4x4 matrix transformation to the argument shader by
// inverting the argument matrix.
func Transform(s glbuild.Shader3D, m ms3.Mat4) (glbuild.Shader3D, error) {
	det := m.Determinant()
	if math32.Abs(det) < 1e-8 {
		return nil, errors.New("singular Mat4")
	}
	return &transform{s: s, invT: m.Inverse()}, nil
}

type transform struct {
	s    glbuild.Shader3D
	invT ms3.Mat4
}

func (u *transform) Bounds() ms3.Box {
	return u.invT.MulBox(u.s.Bounds())
}

func (s *transform) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *transform) AppendShaderName(b []byte) []byte {
	b = append(b, "transform"...)
	// Hash floats so that name is not too long.
	values := s.invT.Array()
	b = fappend(b, hashf(values[:]), 'n', 'd')
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (r *transform) AppendShaderBody(b []byte) []byte {
	b = appendMat4Decl(b, "invT", r.invT)
	b = append(b, "return "...)
	b = r.s.AppendShaderName(b)
	b = append(b, "(((invT) * vec4(p,0.0)).xyz);"...)
	return b
}

// Rotate is the rotation of radians angle around an axis vector.
func Rotate(s glbuild.Shader3D, radians float32, axis ms3.Vec) (glbuild.Shader3D, error) {
	if axis == (ms3.Vec{}) {
		return nil, errors.New("null vector")
	}
	T := ms3.RotationMat4(radians, axis)
	return Transform(s, T)
}

// Translate moves the SDF s in the given direction.
func Translate(s glbuild.Shader3D, dirX, dirY, dirZ float32) glbuild.Shader3D {
	return &translate{s: s, p: ms3.Vec{X: dirX, Y: dirY, Z: dirZ}}
}

type translate struct {
	s glbuild.Shader3D
	p ms3.Vec
}

func (u *translate) Bounds() ms3.Box {
	return u.s.Bounds().Add(u.p)
}

func (s *translate) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *translate) AppendShaderName(b []byte) []byte {
	b = append(b, "translate"...)
	b = vecappend(b, s.p, 0, 'n', 'p')
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *translate) AppendShaderBody(b []byte) []byte {
	b = appendVec3Decl(b, "t", s.p)
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p-t);"...)
	return b
}

// Offset adds sdfAdd to the entire argument SDF. If sdfAdd is negative this will
// round edges and increase the dimension of flat surfaces of the SDF by the absolute magnitude.
// See [Inigo's youtube video] on the subject.
//
// [Inigo's youtube video]: https://www.youtube.com/watch?v=s5NGeUV2EyU
func Offset(s glbuild.Shader3D, sdfAdd float32) glbuild.Shader3D {
	return &offset{s: s, off: sdfAdd}
}

type offset struct {
	s   glbuild.Shader3D
	off float32
}

func (u *offset) Bounds() ms3.Box {
	bb := u.s.Bounds()
	bb.Max = ms3.AddScalar(-u.off, bb.Max)
	bb.Min = ms3.AddScalar(u.off, bb.Min)
	return bb.Canon()
}

func (s *offset) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *offset) AppendShaderName(b []byte) []byte {
	b = append(b, "offset"...)
	b = fappend(b, s.off, 'n', 'p')
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *offset) AppendShaderBody(b []byte) []byte {
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p)+("...)
	b = fappend(b, s.off, '-', '.')
	b = append(b, ')', ';')
	return b
}

// Array is the domain repetition operation. It repeats domain centered around (x,y,z)=(0,0,0)
func Array(s glbuild.Shader3D, spacingX, spacingY, spacingZ float32, nx, ny, nz int) (glbuild.Shader3D, error) {
	if nx <= 0 || ny <= 0 || nz <= 0 {
		return nil, errors.New("invalid array repeat param")
	} else if spacingX <= 0 || spacingY <= 0 || spacingZ <= 0 {
		return nil, errors.New("invalid array spacing")
	}
	return &array{s: s, d: ms3.Vec{X: spacingX, Y: spacingY, Z: spacingZ}, nx: nx, ny: ny, nz: nz}, nil
}

type array struct {
	s          glbuild.Shader3D
	d          ms3.Vec
	nx, ny, nz int
}

func (u *array) Bounds() ms3.Box {
	size := ms3.MulElem(u.nvec3(), u.d)
	bb := ms3.Box{Max: size}
	halfd := ms3.Scale(0.5, u.d)
	offset := ms3.Sub(halfd, size)
	return bb.Add(offset)
}

func (s *array) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *array) AppendShaderName(b []byte) []byte {
	b = append(b, "repeat"...)
	b = vecappend(b, s.d, 'q', 'n', 'p')
	b = vecappend(b, s.nvec3(), 'q', 'n', 'p')
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *array) nvec3() ms3.Vec { return ms3.Vec{X: float32(s.nx), Y: float32(s.ny), Z: float32(s.nz)} }

func (s *array) AppendShaderBody(b []byte) []byte {
	sdf := string(s.s.AppendShaderName(nil))
	// id is the tile index in 3 directions.
	// o is neighbor offset direction (which neighboring tile is closest in 3 directions)
	// s is scaling factors in 3 directions.
	// rid is the neighboring tile index, which is then corrected for limited repetition using clamp.
	body := fmt.Sprintf(`
vec3 s = vec3(%f,%f,%f);
vec3 n = vec3(%d.,%d.,%d.);
vec3 minlim = vec3(0.,0.,0.);
vec3 id = round(p/s);
vec3 o = sign(p-s*id);
float d = %f;
for( int k=0; k<2; k++ )
for( int j=0; j<2; j++ )
for( int i=0; i<2; i++ )
{
	vec3 rid = id + vec3(i,j,k)*o;
	// limited repetition
	rid = clamp(rid, minlim, n);
	vec3 r = p - s*rid;
	d = min( d, %s(r) );
}
return d;`, s.d.X, s.d.Y, s.d.Z,
		s.nx-1, s.ny-1, s.nz-1,
		largenum, sdf,
	)
	b = append(b, body...)
	return b
}

// SmoothUnion joins the shapes of two shaders into one with a smoothing blend.
func SmoothUnion(s1, s2 glbuild.Shader3D, k float32) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		panic("nil object")
	}
	return &smoothUnion{union: union{s1: s1, s2: s2}, k: k}
}

type smoothUnion struct {
	union
	k float32
}

func (s *smoothUnion) AppendShaderName(b []byte) []byte {
	b = append(b, "smoothUnion_"...)
	b = fappend(b, s.k, 'n', 'd')
	b = append(b, '_')
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *smoothUnion) AppendShaderBody(b []byte) []byte {
	b = appendDistanceDecl(b, s.s1, "d1", "p")
	b = appendDistanceDecl(b, s.s2, "d2", "p")
	b = appendFloatDecl(b, "k", s.k)
	b = append(b, `float h = clamp( 0.5 + 0.5*(d2-d1)/k, 0.0, 1.0 );
return mix( d2, d1, h ) - k*h*(1.0-h);`...)
	return b
}

// SmoothDifference performs the difference of two SDFs with a smoothing parameter.
func SmoothDifference(s1, s2 glbuild.Shader3D, k float32) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		panic("nil object")
	}
	return &smoothDiff{diff: diff{s1: s1, s2: s2}, k: k}
}

type smoothDiff struct {
	diff
	k float32
}

func (s *smoothDiff) AppendShaderName(b []byte) []byte {
	b = append(b, "smoothDiff"...)
	b = fappend(b, s.k, 'n', 'd')
	b = append(b, '_')
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *smoothDiff) AppendShaderBody(b []byte) []byte {
	b = appendDistanceDecl(b, s.s1, "d1", "p")
	b = appendDistanceDecl(b, s.s2, "d2", "p")
	b = appendFloatDecl(b, "k", s.k)
	b = append(b, `float h = clamp( 0.5 - 0.5*(d2+d1)/k, 0.0, 1.0 );
return mix( d1, -d2, h ) + k*h*(1.0-h);`...)
	return b
}

// SmoothIntersect performs the intesection of two SDFs with a smoothing parameter.
func SmoothIntersect(s1, s2 glbuild.Shader3D, k float32) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		panic("nil object")
	}
	return &smoothIntersect{intersect: intersect{s1: s1, s2: s2}, k: k}
}

type smoothIntersect struct {
	intersect
	k float32
}

func (s *smoothIntersect) AppendShaderName(b []byte) []byte {
	b = append(b, "smoothIntersect"...)
	b = fappend(b, s.k, 'n', 'd')
	b = append(b, '_')
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *smoothIntersect) AppendShaderBody(b []byte) []byte {
	b = appendDistanceDecl(b, s.s1, "d1", "p")
	b = appendDistanceDecl(b, s.s2, "d2", "p")
	b = appendFloatDecl(b, "k", s.k)
	b = append(b, `float h = clamp( 0.5 - 0.5*(d2-d1)/k, 0.0, 1.0 );
return mix( d2, d1, h ) + k*h*(1.0-h);`...)
	return b
}

// Elongate "stretches" the SDF in a direction by splitting it on the origin in
// the plane perpendicular to the argument direction. The part of the shape in the negative
// plane is discarded and replaced with the elongated positive part.
//
// Arguments are distances, so zero-valued arguments are no-op.
func Elongate(s glbuild.Shader3D, dirX, dirY, dirZ float32) glbuild.Shader3D {
	return &elongate{s: s, h: ms3.Vec{X: dirX, Y: dirY, Z: dirZ}}
}

type elongate struct {
	s glbuild.Shader3D
	h ms3.Vec
}

func (u *elongate) Bounds() ms3.Box {
	box := u.s.Bounds()
	// Elongate splits shape around origin and keeps positive bits only.
	box.Max = ms3.MaxElem(box.Max, ms3.Vec{})
	box.Max = ms3.Add(box.Max, ms3.Scale(0.5, u.h))
	box.Min = ms3.Scale(-1, box.Max) // Discard negative side of shape.
	return box
}

func (s *elongate) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *elongate) AppendShaderName(b []byte) []byte {
	b = append(b, "elongate"...)
	b = vecappend(b, s.h, 'i', 'n', 'p')
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *elongate) AppendShaderBody(b []byte) []byte {
	b = appendVec3Decl(b, "h", ms3.Scale(0.5, s.h))
	b = append(b, "vec3 q = abs(p)-h;"...)
	b = appendDistanceDecl(b, s.s, "d", "max(q,0.0)")
	b = append(b, "return d + min(max(q.x,max(q.y,q.z)),0.0);"...)
	return b
}

// Shell carves the interior of the SDF leaving only the exterior shell of the part.
func Shell(s glbuild.Shader3D, thickness float32) glbuild.Shader3D {
	return &shell{s: s, thick: thickness}
}

type shell struct {
	s     glbuild.Shader3D
	thick float32
}

func (u *shell) Bounds() ms3.Box {
	bb := u.s.Bounds()
	return bb
}

func (s *shell) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *shell) AppendShaderName(b []byte) []byte {
	b = append(b, "shell"...)
	b = fappend(b, s.thick, 'n', 'p')
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *shell) AppendShaderBody(b []byte) []byte {
	b = appendFloatDecl(b, "t", s.thick)
	b = append(b, "return t*(abs("...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p/t))-t);"...)
	return b
}

// circularArray is the circular domain repetition operation around Z axis.
// It repeats domain centered around (x,y)=(0,0) around the Z axis.
func circularArray(s glbuild.Shader3D, angle float32, n int) (glbuild.Shader3D, error) {
	if n <= 0 {
		return nil, errors.New("invalid circarray repeat param")
	} else if s == nil {
		return nil, errors.New("nil argument to circarray")
	}
	return nil, errors.New("TODO")
	return &circarray{s: s, n: n, angle: angle}, nil
}

type circarray struct {
	s     glbuild.Shader3D
	n     int
	angle float32
}

func (ca *circarray) Bounds() ms3.Box {
	// Naive solution, place bounding box N times
	// and take the union of all bounds.
	bb := ca.s.Bounds()
	size := bb.Size()
	center := bb.Center()
	v := ms2.Vec{X: center.X, Y: center.Y}
	m := ms2.RotationMat2(ca.angle)
	for i := 0; i < ca.n; i++ {
		v = ms2.MulMatVec(m, v)
		centerV := ms3.Vec{X: v.X, Y: v.Y, Z: center.Z}
		bb = bb.Union(ms3.NewCenteredBox(centerV, size))
	}
	return bb
}

func (ca *circarray) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &ca.s)
}

// func (ca *circarray) angle() float32 { return 2 * math32.Pi / float32(ca.n) }

func (ca *circarray) AppendShaderName(b []byte) []byte {
	b = append(b, "circarray"...)
	b = fappend(b, float32(ca.n), 'n', 'p')
	b = fappend(b, ca.angle, 'n', 'p')
	b = append(b, '_')
	b = ca.s.AppendShaderName(b)
	return b
}

func (ca *circarray) AppendShaderBody(b []byte) []byte {

	return b
}
