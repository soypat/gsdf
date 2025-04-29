package gsdf

import (
	"fmt"
	"strconv"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/glbuild/glsllib"
)

// OpUnion is the result of the [Union] operation. Prefer using [Union] to using this type directly.
//
// Normally primitives and results of operations in this package are
// not exported since their concrete type provides relatively little value.
// The result of Union is the exception to the rule since it is the
// most common operation to perform on SDFs and can provide
// several benefits to users seeking to optimize their SDFs
// creatively such as creating sectioned SDFs where conditional evaluation
// may be performed depending on the bounding boxes of the SDFs being evaluated.
//
// By exporting OpUnion users can traverse a [glbuild.Shader3D] tree looking for
// OpUnion elements and checking how heavy their computation cost is and
// evaluating if sectioning their bounding box is effective.
type OpUnion struct {
	// joined contains 2 or more 3D SDFs.
	// OpUnion methods will panic if joined less than 2 elements.
	joined []glbuild.Shader3D
}

// Union joins the shapes of several 3D SDFs into one. Is exact.
// Union aggregates nested Union results into its own. To prevent this behaviour use [OpUnion] directly.
func (bld *Builder) Union(shaders ...glbuild.Shader3D) glbuild.Shader3D {
	if len(shaders) < 2 {
		panic("need at least 2 arguments to Union")
	}
	var U OpUnion
	for i, s := range shaders {
		if s == nil {
			bld.nilsdf(fmt.Sprintf("nil arg[%d] to Union", i))
		}
		if subU, ok := s.(*OpUnion); ok {
			// Discard nested union elements and join their elements.
			// Results in much smaller and readable GLSL code.
			U.joined = append(U.joined, subU.joined...)
		} else {
			U.joined = append(U.joined, s)
		}
	}
	return &U
}

// Bounds returns the union of all joined SDFs. Implements [glbuild.Shader3D] and [gleval.SDF3].
func (u *OpUnion) Bounds() ms3.Box {
	u.mustValidate()
	bb := u.joined[0].Bounds()
	for _, bb2 := range u.joined[1:] {
		bb = bb.Union(bb2.Bounds())
	}
	return bb
}

// ForEachChild implements [glbuild.Shader3D].
func (u *OpUnion) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	u.mustValidate()
	for i := range u.joined {
		err := fn(userData, &u.joined[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// AppendShaderName implements [glbuild.Shader].
func (u *OpUnion) AppendShaderName(b []byte) []byte {
	u.mustValidate()
	b = append(b, "union_"...)
	// startNames := len(b)
	for i := range u.joined {
		b = u.joined[i].AppendShaderName(b)
		if i < len(u.joined)-1 {
			b = append(b, '_')
		}
	}
	return b
}

// AppendShaderBody implements [glbuild.Shader].
func (u *OpUnion) AppendShaderBody(b []byte) []byte {
	u.mustValidate()
	b = glbuild.AppendDistanceDecl(b, "d", "p", u.joined[0])
	for i := range u.joined[1:] {
		b = append(b, "d=min(d,"...)
		b = u.joined[i+1].AppendShaderName(b)
		b = append(b, "(p));\n"...)
	}
	b = append(b, "return d;"...)
	return b
}

// AppendShaderObjects implements [glbuild.Shader]. This method returns the argument buffer with no modifications. See [glbuild.Shader] for more information.
func (u *OpUnion) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	u.mustValidate()
	return objects
}

func (u *OpUnion) mustValidate() {
	if len(u.joined) < 2 {
		panic("OpUnion must have at least 2 elements. please prefer using gsdf.Union over gsdf.OpUnion")
	}
}

// Difference is the SDF difference of a-b. Does not produce a true SDF.
func (bld *Builder) Difference(a, b glbuild.Shader3D) glbuild.Shader3D {
	if a == nil || b == nil {
		bld.nilsdf("Difference")
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
	b = glbuild.AppendDistanceDecl(b, "a", "p", s.s1)
	b = glbuild.AppendDistanceDecl(b, "b", "p", s.s2)
	b = append(b, "return max(a,-b);"...)
	return b
}

func (u *diff) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Intersection is the SDF intersection of a ^ b. Does not produce an exact SDF.
func (bld *Builder) Intersection(a, b glbuild.Shader3D) glbuild.Shader3D {
	if a == nil || b == nil {
		bld.nilsdf("Intersection")
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

func (u *intersect) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Xor is the mutually exclusive boolean operation and results in an exact SDF.
func (bld *Builder) Xor(s1, s2 glbuild.Shader3D) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		bld.nilsdf("nil argument to Xor")
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
	b = glbuild.AppendDistanceDecl(b, "d1", "(p)", s.s1)
	b = glbuild.AppendDistanceDecl(b, "d2", "(p)", s.s2)
	b = append(b, "return max(min(d1,d2),-max(d1,d2));"...)
	return b
}

func (u *xor) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Scale scales s by scaleFactor around the origin.
func (bld *Builder) Scale(s glbuild.Shader3D, scaleFactor float32) glbuild.Shader3D {
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
	b = glbuild.AppendFloatDecl(b, "s", s.scale)
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p/s)*s;"...)
	return b
}

func (u *scale) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Symmetry reflects the SDF around one or more cartesian planes.
func (bld *Builder) Symmetry(s glbuild.Shader3D, mirrorX, mirrorY, mirrorZ bool) glbuild.Shader3D {
	if !mirrorX && !mirrorY && !mirrorZ {
		bld.shapeErrorf("ineffective symmetry")
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
	b = s.xyz.AppendMapped_XYZ(b)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *symmetry) AppendShaderBody(b []byte) []byte {
	b = append(b, "p."...)
	b = s.xyz.AppendMapped_xyz(b)
	b = append(b, "=abs(p."...)
	b = s.xyz.AppendMapped_xyz(b)
	b = append(b, ");\n return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p);"...)
	return b
}

func (u *symmetry) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Transform applies a 4x4 matrix transformation to the argument shader by
// inverting the argument matrix.
func (bld *Builder) Transform(s glbuild.Shader3D, m ms3.Mat4) glbuild.Shader3D {
	det := m.Determinant()
	if math32.Abs(det) < epstol {
		bld.shapeErrorf("singular Mat4")
	}
	t := &transform{s: s, t: m, tInv: m.Inverse()}
	t.hash = hashshaderptr(t) // TODO(soypat): fix TestTransformDuplicateBug so that hashing input shader works.
	return t
}

type transform struct {
	s glbuild.Shader3D
	// Transformation matrix. Transforms points. We use it
	// to transform the bounding box.
	t ms3.Mat4 // The actual transformation matrix,
	// Inverse transformation matrix needed for SDF.
	// The SDF receives points which we must evaluate in
	// transformed coordinates, so we must work backwards, thus inverse.
	tInv ms3.Mat4
	hash uint64
}

func (t *transform) Bounds() ms3.Box {
	return t.t.MulBox(t.s.Bounds())
}

func (t *transform) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &t.s)
}

func (t *transform) AppendShaderName(b []byte) []byte {
	b = append(b, "transform"...)

	// Hash floats so that name is not too long.
	values := t.t.Array()
	b = glbuild.AppendFloat(b, 'n', 'p', hashf(values[:]))
	b = append(b, '_')
	b = strconv.AppendUint(b, t.hash, 32)
	return b
}

func (t *transform) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendMat4Decl(b, "invT", t.tInv)
	b = append(b, "return "...)
	b = t.s.AppendShaderName(b)
	b = append(b, "(((invT) * vec4(p,0.0)).xyz);"...)
	return b
}

func (t *transform) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Rotate is the rotation of radians angle around an axis vector.
func (bld *Builder) Rotate(s glbuild.Shader3D, radians float32, axis ms3.Vec) glbuild.Shader3D {
	if axis == (ms3.Vec{}) {
		bld.shapeErrorf("null vector")
	}
	T := ms3.RotatingMat4(radians, axis)
	return bld.Transform(s, T)
}

// Translate moves the SDF s in the given direction (dirX, dirY, dirZ) and returns the result.
func (bld *Builder) Translate(s glbuild.Shader3D, dirX, dirY, dirZ float32) glbuild.Shader3D {
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
	arr := s.p.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *translate) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec3Decl(b, "t", s.p)
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p-t);"...)
	return b
}

func (u *translate) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Offset adds sdfAdd to the entire argument SDF. If sdfAdd is negative this will
// round edges and increase the dimension of flat surfaces of the SDF by the absolute magnitude.
// See [Inigo's youtube video] on the subject.
//
// [Inigo's youtube video]: https://www.youtube.com/watch?v=s5NGeUV2EyU
func (bld *Builder) Offset(s glbuild.Shader3D, sdfAdd float32) glbuild.Shader3D {
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
	b = glbuild.AppendFloat(b, 'n', 'p', s.off)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *offset) AppendShaderBody(b []byte) []byte {
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p)+("...)
	b = glbuild.AppendFloat(b, '-', '.', s.off)
	b = append(b, ')', ';')
	return b
}

func (u *offset) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Array is the domain repetition operation. It repeats domain centered around the origin (x,y,z)=(0,0,0).
func (bld *Builder) Array(s glbuild.Shader3D, spacingX, spacingY, spacingZ float32, nx, ny, nz int) glbuild.Shader3D {
	if nx <= 0 || ny <= 0 || nz <= 0 {
		bld.shapeErrorf("invalid array repeat param")
	}
	if spacingX <= 0 || spacingY <= 0 || spacingZ <= 0 {
		bld.shapeErrorf("invalid array spacing")
	}
	return &array{s: s, d: ms3.Vec{X: spacingX, Y: spacingY, Z: spacingZ}, nx: nx, ny: ny, nz: nz}
}

type array struct {
	s          glbuild.Shader3D
	d          ms3.Vec
	nx, ny, nz int
}

func (u *array) Bounds() ms3.Box {
	// TODO(soypat): use more accurate algorithm for bounds calculation.
	sbb := u.s.Bounds()
	size := ms3.MulElem(u.nvec3(), u.d)
	sbb.Max = ms3.Add(sbb.Max, size)
	return sbb
}

func (s *array) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &s.s)
}

func (s *array) AppendShaderName(b []byte) []byte {
	b = append(b, "repeat"...)
	arr := s.d.Array()
	b = glbuild.AppendFloats(b, 'q', 'n', 'p', arr[:]...)
	arr = s.nvec3().Array()
	b = glbuild.AppendFloats(b, 'q', 'n', 'p', arr[:]...)
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
	b = fmt.Appendf(b, `
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
		largenum, sdf)
	return b
}

func (u *array) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// SmoothUnion joins the shapes of two shaders into one with a smoothing blend.
func (bld *Builder) SmoothUnion(k float32, s1, s2 glbuild.Shader3D) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		bld.nilsdf("SmoothUnion")
	}
	return &smoothUnion{s1: s1, s2: s2, k: k}
}

type smoothUnion struct {
	s1, s2 glbuild.Shader3D
	k      float32
}

func (s *smoothUnion) Bounds() ms3.Box {
	return s.s1.Bounds().Union(s.s2.Bounds())
}

func (s *smoothUnion) ForEachChild(userData any, fn func(any, *glbuild.Shader3D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *smoothUnion) AppendShaderName(b []byte) []byte {
	b = append(b, "smoothUnion_"...)
	b = glbuild.AppendFloat(b, 'n', 'd', s.k)
	b = append(b, '_')
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *smoothUnion) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendDistanceDecl(b, "d1", "p", s.s1)
	b = glbuild.AppendDistanceDecl(b, "d2", "p", s.s2)
	b = glbuild.AppendFloatDecl(b, "k", s.k)
	b = append(b, `float h = clamp( 0.5 + 0.5*(d2-d1)/k, 0.0, 1.0 );
return mix( d2, d1, h ) - k*h*(1.0-h);`...)
	return b
}

func (u *smoothUnion) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// SmoothDifference performs the difference of two SDFs with a smoothing parameter.
func (bld *Builder) SmoothDifference(k float32, s1, s2 glbuild.Shader3D) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		bld.nilsdf("SmoothDifference")
	}
	return &smoothDiff{diff: diff{s1: s1, s2: s2}, k: k}
}

type smoothDiff struct {
	diff
	k float32
}

func (s *smoothDiff) AppendShaderName(b []byte) []byte {
	b = append(b, "smoothDiff"...)
	b = glbuild.AppendFloat(b, 'n', 'd', s.k)
	b = append(b, '_')
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *smoothDiff) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendDistanceDecl(b, "d1", "p", s.s1)
	b = glbuild.AppendDistanceDecl(b, "d2", "p", s.s2)
	b = glbuild.AppendFloatDecl(b, "k", s.k)
	b = append(b, `float h = clamp( 0.5 - 0.5*(d2+d1)/k, 0.0, 1.0 );
return mix( d1, -d2, h ) + k*h*(1.0-h);`...)
	return b
}

// SmoothIntersect performs the intesection of two SDFs with a smoothing parameter.
func (bld *Builder) SmoothIntersect(k float32, s1, s2 glbuild.Shader3D) glbuild.Shader3D {
	if s1 == nil || s2 == nil {
		bld.nilsdf("SmoothIntersect")
	}
	return &smoothIntersect{intersect: intersect{s1: s1, s2: s2}, k: k}
}

type smoothIntersect struct {
	intersect
	k float32
}

func (s *smoothIntersect) AppendShaderName(b []byte) []byte {
	b = append(b, "smoothIntersect"...)
	b = glbuild.AppendFloat(b, 'n', 'd', s.k)
	b = append(b, '_')
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *smoothIntersect) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendDistanceDecl(b, "d1", "p", s.s1)
	b = glbuild.AppendDistanceDecl(b, "d2", "p", s.s2)
	b = glbuild.AppendFloatDecl(b, "k", s.k)
	b = append(b, `float h = clamp( 0.5 - 0.5*(d2-d1)/k, 0.0, 1.0 );
return mix( d2, d1, h ) + k*h*(1.0-h);`...)
	return b
}

// Elongate "stretches" the SDF in a direction by splitting it on the origin in
// the plane perpendicular to the argument direction. The part of the shape in the negative
// plane is discarded and replaced with the elongated positive part.
//
// Arguments are distances, so zero-valued arguments are no-op.
func (bld *Builder) Elongate(s glbuild.Shader3D, dirX, dirY, dirZ float32) glbuild.Shader3D {
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
	arr := s.h.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *elongate) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec3Decl(b, "h", ms3.Scale(0.5, s.h))
	b = append(b, "vec3 q=abs(p)-h;"...)
	b = glbuild.AppendDistanceDecl(b, "d", "max(q,0.)", s.s)
	b = append(b, "return d+min(max(q.x,max(q.y,q.z)),0.);"...)
	return b
}

func (u *elongate) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Shell carves the interior of the SDF leaving only the exterior shell of the part.
func (bld *Builder) Shell(s glbuild.Shader3D, thickness float32) glbuild.Shader3D {
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
	b = glbuild.AppendFloat(b, 'n', 'p', s.thick)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *shell) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "t", s.thick)
	b = append(b, "return t*(abs("...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p/t))-t);"...)
	return b
}

func (u *shell) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// CircularArray is the circular domain repetition operation around the origin (x,y,z)=(0,0,0).
// It repeats the shape numInstances times and the spacing angle is defined by circleDiv such that angle = 2*pi/circleDiv.
// The operation is defined this way so that the argument shape is evaluated only twice per circular array evaluation, regardless of instances.
func (bld *Builder) CircularArray(s glbuild.Shader3D, numInstances, circleDiv int) glbuild.Shader3D {
	if s == nil {
		bld.nilsdf("nil argument to circarray")
	}
	if circleDiv <= 1 || numInstances <= 0 {
		bld.shapeErrorf("invalid circarray repeat param")
	}
	if numInstances > circleDiv {
		bld.shapeErrorf("bad circular array instances, must be less than or equal to circleDiv")
	}
	return &circarray{s: s, circleDiv: circleDiv, nInst: numInstances}
}

type circarray struct {
	s         glbuild.Shader3D
	nInst     int
	circleDiv int
}

func (ca *circarray) Bounds() ms3.Box {
	bb := ca.s.Bounds()
	bb2 := ms2.Box{
		Min: ms2.Vec{X: bb.Min.X, Y: bb.Min.Y},
		Max: ms2.Vec{X: bb.Max.X, Y: bb.Max.Y},
	}
	verts := bb2.Vertices()
	angle := 2 * math32.Pi / float32(ca.circleDiv)
	m := ms2.RotationMat2(angle)
	for i := 0; i < ca.nInst-1; i++ {
		for i := range verts {
			verts[i] = ms2.MulMatVec(m, verts[i])
			bb2 = bb2.IncludePoint(verts[i])
		}
	}
	bb.Max.X, bb.Max.Y = bb2.Max.X, bb2.Max.Y
	bb.Min.X, bb.Min.Y = bb2.Min.X, bb2.Min.Y
	return bb
}

func (ca *circarray) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return fn(userData, &ca.s)
}

// func (ca *circarray) angle() float32 { return 2 * math32.Pi / float32(ca.n) }

func (ca *circarray) AppendShaderName(b []byte) []byte {
	b = append(b, "circarray"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', float32(ca.nInst), float32(ca.circleDiv))
	b = append(b, '_')
	b = ca.s.AppendShaderName(b)
	return b
}

func (ca *circarray) AppendShaderBody(b []byte) []byte {
	angle := 2 * math32.Pi / float32(ca.circleDiv)
	b = glbuild.AppendFloatDecl(b, "ncirc", float32(ca.circleDiv))
	b = glbuild.AppendFloatDecl(b, "angle", angle)
	b = glbuild.AppendFloatDecl(b, "ninsm1", float32(ca.nInst-1))
	b = append(b, `vec4 p0p1 = gsdfPartialCircArray2D(p.xy, ncirc, angle, ninsm1);`...)
	b = glbuild.AppendDistanceDecl(b, "d0", "vec3(p0p1.x,p0p1.y,p.z)", ca.s)
	b = glbuild.AppendDistanceDecl(b, "d1", "vec3(p0p1.z,p0p1.w,p.z)", ca.s)
	b = append(b, "return min(d0, d1);"...)
	return b
}

func (u *circarray) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return append(objects, glsllib.PartialCircArray2D())
}
