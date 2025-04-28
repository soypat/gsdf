package gsdf

import (
	"fmt"
	"math"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/glbuild/glsllib"
)

// OpUnion2D is the result of [Union2D]. This type is exported for special reasons, see [OpUnion] documentation.
type OpUnion2D struct {
	joined []glbuild.Shader2D
}

// Union joins the shapes of several 2D SDFs into one. Is exact.
// Union aggregates nested Union results into its own.
func (*Builder) Union2D(shaders ...glbuild.Shader2D) glbuild.Shader2D {
	if len(shaders) < 2 {
		panic("need at least 2 arguments to Union2D")
	}
	var U OpUnion2D
	for i, s := range shaders {
		if s == nil {
			panic(fmt.Sprintf("nil %d argument to Union2D", i))
		}
		if subU, ok := s.(*OpUnion2D); ok {
			// Discard nested union elements and join their elements.
			// Results in much smaller and readable GLSL code.
			U.joined = append(U.joined, subU.joined...)
		} else {
			U.joined = append(U.joined, s)
		}
	}
	return &U
}

// Bounds returns the union of all joined SDFs. Implements [glbuild.Shader2D] and [gleval.SDF2].
func (u *OpUnion2D) Bounds() ms2.Box {
	u.mustValidate()
	bb := u.joined[0].Bounds()
	for _, bb2 := range u.joined[1:] {
		bb = bb.Union(bb2.Bounds())
	}
	return bb
}

// ForEachChild implements [glbuild.Shader2D].
func (u *OpUnion2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
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
func (u *OpUnion2D) AppendShaderName(b []byte) []byte {
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
func (u *OpUnion2D) AppendShaderBody(b []byte) []byte {
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
func (u *OpUnion2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	u.mustValidate()
	return objects
}

func (u *OpUnion2D) mustValidate() {
	if len(u.joined) < 2 {
		panic("OpUnion2D must have at least 2 elements. Please prefer using gsdf.Union2D over gsdf.OpUnion2D")
	}
}

// Extrude converts a 2D SDF into a 3D extrusion. Extrudes in both positive and negative Z direction, half of h both ways.
func (bld *Builder) Extrude(s glbuild.Shader2D, h float32) glbuild.Shader3D {
	if s == nil {
		bld.nilsdf("Extrude")
	}
	if h < 0 {
		bld.shapeErrorf("bad extrusion length")
	}
	return &extrusion{s: s, h: h}
}

type extrusion struct {
	s glbuild.Shader2D
	h float32
}

func (e *extrusion) Bounds() ms3.Box {
	b2 := e.s.Bounds()
	hd2 := e.h / 2
	return ms3.Box{
		Min: ms3.Vec{X: b2.Min.X, Y: b2.Min.Y, Z: -hd2},
		Max: ms3.Vec{X: b2.Max.X, Y: b2.Max.Y, Z: hd2},
	}
}

func (e *extrusion) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &e.s)
}
func (e *extrusion) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}
func (u *extrusion) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

func (e *extrusion) AppendShaderName(b []byte) []byte {
	b = append(b, "extrusion_"...)
	b = e.s.AppendShaderName(b)
	return b
}

func (e *extrusion) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "h", e.h/2)
	b = glbuild.AppendDistanceDecl(b, "d", "p.xy", e.s)
	b = append(b, `vec2 w = vec2( d, abs(p.z) - h );
return min(max(w.x,w.y),0.0) + length(max(w,0.0));`...)
	return b
}

// Revolve revolves a 2D SDF around the y axis, offsetting the axis of revolution by axisOffset.
func (bld *Builder) Revolve(s glbuild.Shader2D, axisOffset float32) glbuild.Shader3D {
	if s == nil {
		bld.shapeErrorf("nil argument to Revolve")
	}
	if axisOffset < 0 {
		bld.shapeErrorf("negative axis offset")
	}
	return &revolution{s2d: s, off: axisOffset}
}

type revolution struct {
	s2d glbuild.Shader2D
	off float32
}

func (r *revolution) Bounds() ms3.Box {
	b2 := r.s2d.Bounds()
	radius := math32.Max(0, b2.Max.X-r.off)
	return ms3.Box{
		Min: ms3.Vec{X: -radius, Y: b2.Min.Y, Z: -radius},
		Max: ms3.Vec{X: radius, Y: b2.Max.Y, Z: radius}, // TODO
	}
}

func (r *revolution) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &r.s2d)
}
func (r *revolution) ForEachChild(userData any, fn func(userData any, s *glbuild.Shader3D) error) error {
	return nil
}
func (u *revolution) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

func (r *revolution) AppendShaderName(b []byte) []byte {
	b = append(b, "revolution_"...)
	b = r.s2d.AppendShaderName(b)
	return b
}

func (r *revolution) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "w", r.off)
	b = append(b, "vec2 q = vec2( length(p.xz) - w, p.y );\n"...)
	b = glbuild.AppendDistanceDecl(b, "d", "q", r.s2d)
	b = append(b, "return d;"...)
	return b
}

// Difference2D is the SDF difference of a-b. Does not produce a true SDF.
func (bld *Builder) Difference2D(a, b glbuild.Shader2D) glbuild.Shader2D {
	if a == nil || b == nil {
		bld.nilsdf("Difference2D")
	}
	return &diff2D{s1: a, s2: b}
}

type diff2D struct {
	s1, s2 glbuild.Shader2D // Performs s1-s2.
}

func (u *diff2D) Bounds() ms2.Box {
	return u.s1.Bounds()
}

func (s *diff2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *diff2D) AppendShaderName(b []byte) []byte {
	b = append(b, "diff2D_"...)
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *diff2D) AppendShaderBody(b []byte) []byte {
	b = append(b, "return max("...)
	b = s.s1.AppendShaderName(b)
	b = append(b, "(p),-"...)
	b = s.s2.AppendShaderName(b)
	b = append(b, "(p));"...)
	return b
}
func (u *diff2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Intersection2D is the SDF intersection of a ^ b. Does not produce an exact SDF.
func (bld *Builder) Intersection2D(a, b glbuild.Shader2D) glbuild.Shader2D {
	if a == nil || b == nil {
		bld.nilsdf("nil argument to Intersection2D")
	}
	return &intersect2D{s1: a, s2: b}
}

type intersect2D struct {
	s1, s2 glbuild.Shader2D // Performs s1 ^ s2.
}

func (u *intersect2D) Bounds() ms2.Box {
	return u.s1.Bounds().Intersect(u.s2.Bounds())
}

func (s *intersect2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *intersect2D) AppendShaderName(b []byte) []byte {
	b = append(b, "intersect2D_"...)
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *intersect2D) AppendShaderBody(b []byte) []byte {
	b = append(b, "return max("...)
	b = s.s1.AppendShaderName(b)
	b = append(b, "(p),"...)
	b = s.s2.AppendShaderName(b)
	b = append(b, "(p));"...)
	return b
}
func (u *intersect2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Xor2D is the mutually exclusive boolean operation and results in an exact SDF.
func (bld *Builder) Xor2D(s1, s2 glbuild.Shader2D) glbuild.Shader2D {
	if s1 == nil || s2 == nil {
		bld.nilsdf("Xor2D")
	}
	return &xor2D{s1: s1, s2: s2}
}

type xor2D struct {
	s1, s2 glbuild.Shader2D
}

func (u *xor2D) Bounds() ms2.Box {
	return u.s1.Bounds().Union(u.s2.Bounds())
}

func (s *xor2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	err := fn(userData, &s.s1)
	if err != nil {
		return err
	}
	return fn(userData, &s.s2)
}

func (s *xor2D) AppendShaderName(b []byte) []byte {
	b = append(b, "xor2D_"...)
	b = s.s1.AppendShaderName(b)
	b = append(b, '_')
	b = s.s2.AppendShaderName(b)
	return b
}

func (s *xor2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendDistanceDecl(b, "d1", "(p)", s.s1)
	b = glbuild.AppendDistanceDecl(b, "d2", "(p)", s.s2)
	b = append(b, "return max(min(d1,d2),-max(d1,d2));"...)
	return b
}
func (u *xor2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Array is the domain repetition operation. It repeats domain centered around (x,y)=(0,0).
func (bld *Builder) Array2D(s glbuild.Shader2D, spacingX, spacingY float32, nx, ny int) glbuild.Shader2D {
	if nx <= 0 || ny <= 0 {
		bld.shapeErrorf("invalid array repeat param")
	}
	okArray := spacingX > 0 && spacingY > 0 && !math32.IsInf(spacingX, 1) && !math32.IsInf(spacingY, 1)
	if !okArray {
		bld.shapeErrorf("bad array spacing")
	}
	return &array2D{s: s, d: ms2.Vec{X: spacingX, Y: spacingY}, nx: nx, ny: ny}
}

type array2D struct {
	s      glbuild.Shader2D
	d      ms2.Vec
	nx, ny int
}

func (u *array2D) Bounds() ms2.Box {
	// TODO(soypat): use more accurate algorithm for bounds calculation.
	sbb := u.s.Bounds()
	size := ms2.MulElem(u.nvec2(), u.d)
	sbb.Max = ms2.Add(sbb.Max, size)
	return sbb
}

func (s *array2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *array2D) AppendShaderName(b []byte) []byte {
	b = append(b, "array2d"...)
	arr := s.d.Array()
	b = glbuild.AppendFloats(b, 'q', 'n', 'p', arr[:]...)
	arr = s.nvec2().Array()
	b = glbuild.AppendFloats(b, 'q', 'n', 'p', arr[:]...)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *array2D) nvec2() ms2.Vec {
	return ms2.Vec{X: float32(s.nx), Y: float32(s.ny)}
}

func (s *array2D) AppendShaderBody(b []byte) []byte {
	sdf := string(s.s.AppendShaderName(nil))
	// id is the tile index in 3 directions.
	// o is neighbor offset direction (which neighboring tile is closest in 3 directions)
	// s is scaling factors in 3 directions.
	// rid is the neighboring tile index, which is then corrected for limited repetition using clamp.
	b = fmt.Appendf(b, `
vec2 s = vec2(%f,%f);
vec2 n = vec2(%d.,%d.);
vec2 minlim = vec2(0.,0.);
vec2 id = round(p/s);
vec2 o = sign(p-s*id);
float d = %f;
for( int j=0; j<2; j++ )
for( int i=0; i<2; i++ )
{
	vec2 rid = id + vec2(i,j)*o;
	// limited repetition
	rid = clamp(rid, minlim, n);
	vec2 r = p - s*rid;
	d = min( d, %s(r) );
}
return d;`, s.d.X, s.d.Y,
		s.nx-1, s.ny-1,
		largenum, sdf)
	return b
}
func (u *array2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Offset2D adds sdfAdd to the entire argument SDF. If sdfAdd is negative this will
// round edges and increase the dimension of flat surfaces of the SDF by the absolute magnitude.
// See [Inigo's youtube video] on the subject.
//
// [Inigo's youtube video]: https://www.youtube.com/watch?v=s5NGeUV2EyU
func (bld *Builder) Offset2D(s glbuild.Shader2D, sdfAdd float32) glbuild.Shader2D {
	return &offset2D{s: s, f: sdfAdd}
}

type offset2D struct {
	s glbuild.Shader2D
	f float32
}

func (u *offset2D) Bounds() ms2.Box {
	// TODO: this does not seem right. Removing if statement breaks gasket example STL.
	bb := u.s.Bounds()
	if u.f > 0 {
		return bb
	}
	bb.Max = ms2.AddScalar(-u.f, bb.Max)
	bb.Min = ms2.AddScalar(u.f, bb.Min)
	return bb
}

func (s *offset2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *offset2D) AppendShaderName(b []byte) []byte {
	b = append(b, "offset2D"...)
	b = glbuild.AppendFloat(b, 'n', 'p', s.f)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *offset2D) AppendShaderBody(b []byte) []byte {
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p)+("...)
	b = glbuild.AppendFloat(b, '-', '.', s.f)
	b = append(b, ')', ';')
	return b
}
func (u *offset2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Translate2D moves the SDF s in the given direction.
func (bld *Builder) Translate2D(s glbuild.Shader2D, dirX, dirY float32) glbuild.Shader2D {
	return &translate2D{s: s, p: ms2.Vec{X: dirX, Y: dirY}}
}

type translate2D struct {
	s glbuild.Shader2D
	p ms2.Vec
}

func (u *translate2D) Bounds() ms2.Box {
	return u.s.Bounds().Add(u.p)
}

func (s *translate2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *translate2D) AppendShaderName(b []byte) []byte {
	b = append(b, "translate2D"...)
	arr := s.p.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *translate2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2Decl(b, "t", s.p)
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p-t);"...)
	return b
}
func (u *translate2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Rotate2D returns the argument shape rotated around the origin by theta (radians).
func (bld *Builder) Rotate2D(s glbuild.Shader2D, theta float32) glbuild.Shader2D {
	m := ms2.RotationMat2(theta)
	det := m.Determinant()
	if math32.Abs(det) < epstol {
		bld.shapeErrorf("badly conditioned rotation")
	}
	return &rotation2D{
		s:    s,
		t:    m,
		tInv: m.Inverse(),
	}
}

type rotation2D struct {
	s    glbuild.Shader2D
	t    ms2.Mat2
	tInv ms2.Mat2
}

func (u *rotation2D) Bounds() ms2.Box {
	bb := u.s.Bounds()
	verts := bb.Vertices()
	v1 := ms2.MulMatVec(u.t, verts[0])
	bb.Max = v1
	bb.Min = v1
	for _, v := range verts[1:] {
		v = ms2.MulMatVec(u.t, v)
		bb.Max = ms2.MaxElem(bb.Max, v)
		bb.Min = ms2.MinElem(bb.Min, v)
	}
	return bb
}

func (s *rotation2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *rotation2D) AppendShaderName(b []byte) []byte {
	b = append(b, "rotation2D"...)
	// Hash floats so that name is not too long.
	values := s.t.Array()
	b = glbuild.AppendFloat(b, 'p', 'n', hashf(values[:]))
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (r *rotation2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendMat2Decl(b, "invT", r.tInv)
	b = append(b, "return "...)
	b = r.s.AppendShaderName(b)
	b = append(b, "(invT * p);"...)
	return b
}

func (u *rotation2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Symmetry reflects the SDF around x or y (or both) axis.
func (bld *Builder) Symmetry2D(s glbuild.Shader2D, mirrorX, mirrorY bool) glbuild.Shader2D {
	if !mirrorX && !mirrorY {
		bld.shapeErrorf("ineffective symmetry")
	}
	return &symmetry2D{s: s, xy: glbuild.NewXYZBits(mirrorX, mirrorY, false)}
}

type symmetry2D struct {
	s  glbuild.Shader2D
	xy glbuild.XYZBits
}

func (u *symmetry2D) Bounds() ms2.Box {
	box := u.s.Bounds()
	if u.xy.X() {
		box.Min.X = minf(box.Min.X, -box.Max.X)
	}
	if u.xy.Y() {
		box.Min.Y = minf(box.Min.Y, -box.Max.Y)
	}
	return box
}

func (s *symmetry2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *symmetry2D) AppendShaderName(b []byte) []byte {
	b = append(b, "symmetry2D"...)
	b = s.xy.AppendMapped_XYZ(b)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *symmetry2D) AppendShaderBody(b []byte) []byte {
	b = append(b, "p."...)
	b = s.xy.AppendMapped_xyz(b)
	b = append(b, "=abs(p."...)
	b = s.xy.AppendMapped_xyz(b)
	b = append(b, ");\n return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p);"...)
	return b
}

func (u *symmetry2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// Annulus makes a 2D shape annular by emptying it's center. It is the equivalent of the 3D Shell operation but in 2D.
func (bld *Builder) Annulus(s glbuild.Shader2D, sub float32) glbuild.Shader2D {
	if s == nil {
		bld.nilsdf("Annulus")
	}
	if sub <= 0 {
		bld.shapeErrorf("invalid annular parameter")
	}
	return &annulus2D{s: s, r: sub}
}

type annulus2D struct {
	s glbuild.Shader2D
	r float32
}

func (u *annulus2D) Bounds() ms2.Box {
	return u.s.Bounds()
}

func (s *annulus2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *annulus2D) AppendShaderName(b []byte) []byte {
	b = append(b, "annulus"...)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *annulus2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "r", s.r)
	b = glbuild.AppendDistanceDecl(b, "d", "p", s.s)
	b = append(b, "return abs(d)-r;"...)
	return b
}

func (u *annulus2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// CircularArray2D is the circular domain repetition operation around the origin (x,y)=(0,0).
// It repeats the shape numInstances times and the spacing angle is defined by circleDiv such that angle = 2*pi/circleDiv.
// The operation is defined this way so that the argument shape is evaluated only twice per circular array evaluation, regardless of instances.
func (bld *Builder) CircularArray2D(s glbuild.Shader2D, numInstances, circleDiv int) glbuild.Shader2D {
	if s == nil {
		bld.nilsdf("circarray2D")
	}
	if circleDiv <= 1 || numInstances <= 0 {
		bld.shapeErrorf("invalid circarray repeat param")
	}
	if numInstances > circleDiv {
		bld.shapeErrorf("bad circular array instances, must be less than or equal to circleDiv")
	}
	return &circarray2D{s: s, circleDiv: circleDiv, nInst: numInstances}
}

type circarray2D struct {
	s         glbuild.Shader2D
	nInst     int
	circleDiv int
}

func (ca *circarray2D) Bounds() ms2.Box {
	bb := ca.s.Bounds()
	verts := bb.Vertices()
	angle := 2 * math.Pi / float32(ca.circleDiv)
	m := ms2.RotationMat2(angle)
	for i := 0; i < ca.nInst-1; i++ {
		for i := range verts {
			verts[i] = ms2.MulMatVec(m, verts[i])
			bb = bb.IncludePoint(verts[i])
		}
	}
	return bb
}

func (ca *circarray2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &ca.s)
}

// func (ca *circarray2D) angle() float32 { return 2 * math32.Pi / float32(ca.n) }

func (ca *circarray2D) AppendShaderName(b []byte) []byte {
	b = append(b, "circarray2D"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', float32(ca.nInst), float32(ca.circleDiv))
	b = append(b, '_')
	b = ca.s.AppendShaderName(b)
	return b
}

func (ca *circarray2D) AppendShaderBody(b []byte) []byte {
	angle := 2 * math.Pi / float32(ca.circleDiv)
	b = glbuild.AppendFloatDecl(b, "ncirc", float32(ca.circleDiv))
	b = glbuild.AppendFloatDecl(b, "angle", angle)
	b = glbuild.AppendFloatDecl(b, "ninsm1", float32(ca.nInst-1))
	b = append(b, `vec4 p0p1 = gsdfPartialCircArray2D(p,ncirc,angle,ninsm1);`...)
	b = glbuild.AppendDistanceDecl(b, "d0", "p0p1.xy", ca.s)
	b = glbuild.AppendDistanceDecl(b, "d1", "p0p1.zw", ca.s)
	b = append(b, "return min(d0, d1);"...)
	return b
}

func (u *circarray2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return append(objects, glsllib.PartialCircArray2D())
}

// ScaleXY scales s by scaleFactor around the origin.
func (bld *Builder) Scale2D(s glbuild.Shader2D, scale float32) glbuild.Shader2D {
	return &scale2D{s: s, scale: scale}
}

type scale2D struct {
	s     glbuild.Shader2D
	scale float32
}

func (u *scale2D) Bounds() ms2.Box {
	b := u.s.Bounds()
	return b.Scale(ms2.Vec{X: u.scale, Y: u.scale})
}

func (s *scale2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *scale2D) AppendShaderName(b []byte) []byte {
	b = append(b, "scalexy_"...)
	b = s.s.AppendShaderName(b)
	return b
}

func (s *scale2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "s", s.scale)
	b = append(b, "return "...)
	b = s.s.AppendShaderName(b)
	b = append(b, "(p/s)*s;"...)
	return b
}

func (u *scale2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// TranslateMulti2D displaces N instances of s SDF to positions given by displacements of length N.
func (bld *Builder) TranslateMulti2D(s glbuild.Shader2D, displacements []ms2.Vec) glbuild.Shader2D {
	if s == nil {
		bld.nilsdf("TranslateMulti2D")
	}
	return &translateMulti2D{
		displacements: displacements,
		s:             s,
		bufname:       makeHashName(nil, "translateMulti2D", displacements),
	}
}

type translateMulti2D struct {
	displacements []ms2.Vec
	s             glbuild.Shader2D
	bufname       []byte
}

func (tm *translateMulti2D) Bounds() ms2.Box {
	var bb ms2.Box
	elemBox := tm.s.Bounds()
	for i := range tm.displacements {
		bb = bb.Union(elemBox.Add(tm.displacements[i]))
	}
	return bb
}

func (tm *translateMulti2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &tm.s)
}

func (tm *translateMulti2D) AppendShaderName(b []byte) []byte {
	b = append(b, "translateMulti2D_"...)
	b = tm.s.AppendShaderName(b)
	return b
}

func (tm *translateMulti2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendDefineDecl(b, "v", string(tm.bufname))
	b = fmt.Appendf(b,
		`const int num = v.length();
	float d = 1.0e23;
	for( int i=0; i<num; i++ )
	{
		vec2 pt = p - v[i];
		d = min(d, %s(pt));
	}
	return d;
`, tm.s.AppendShaderName(nil))
	b = glbuild.AppendUndefineDecl(b, "v")
	return b
}

func (tm *translateMulti2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	ssbo, err := glbuild.MakeShaderBufferReadOnly(tm.bufname, tm.displacements)
	if err != nil {
		panic(err)
	}
	return append(objects, ssbo)
}

type elongate2D struct {
	s glbuild.Shader2D
	h ms2.Vec
}

// Elongate2D "stretches" the SDF in a direction by splitting it on the origin in
// the plane perpendicular to the argument direction. The part of the shape in the negative
// plane is discarded and replaced with the elongated positive part.
//
// Arguments are distances, so zero-valued arguments are no-op.
func (bld *Builder) Elongate2D(s glbuild.Shader2D, dirX, dirY float32) glbuild.Shader2D {
	return &elongate2D{s: s, h: ms2.Vec{X: dirX, Y: dirY}}
}

func (u *elongate2D) Bounds() ms2.Box {
	box := u.s.Bounds()
	// elongate2D splits shape around origin and keeps positive bits only.
	box.Max = ms2.MaxElem(box.Max, ms2.Vec{})
	box.Max = ms2.Add(box.Max, ms2.Scale(0.5, u.h))
	box.Min = ms2.Scale(-1, box.Max) // Discard negative side of shape.
	return box
}

func (s *elongate2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return fn(userData, &s.s)
}

func (s *elongate2D) AppendShaderName(b []byte) []byte {
	b = append(b, "elongate2D"...)
	arr := s.h.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	b = append(b, '_')
	b = s.s.AppendShaderName(b)
	return b
}

func (s *elongate2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2Decl(b, "h", ms2.Scale(0.5, s.h))
	b = append(b, "vec2 q=abs(p)-h;"...)
	b = glbuild.AppendDistanceDecl(b, "d", "max(q,0.)", s.s)
	b = append(b, "return d+min(max(q.x,q.y),0.);"...)
	return b
}

func (u *elongate2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}
