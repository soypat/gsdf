package gsdf

import (
	"fmt"
	"math"
	"strconv"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/glbuild"
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

// NewLine2D creates a straight line between (x0,y0) and (x1,y1) with a given thickness.
func (bld *Builder) NewLine2D(x0, y0, x1, y1, width float32) glbuild.Shader2D {
	hasNaN := math32.IsNaN(x0) || math32.IsNaN(y0) || math32.IsNaN(x1) || math32.IsNaN(y1) || math32.IsNaN(width)
	if hasNaN {
		bld.shapeErrorf("NaN argument to NewLine2D")
	} else if width < 0 {
		bld.shapeErrorf("negative thickness to NewLine2D")
	}
	a, b := ms2.Vec{X: x0, Y: y0}, ms2.Vec{X: x1, Y: y1}
	lineLen := ms2.Norm(ms2.Sub(a, b))
	if lineLen < width*1e-6 || lineLen < epstol {
		if width == 0 {
			bld.shapeErrorf("infimal line")
		}
		return bld.NewCircle(width / 2)
	}
	return &line2D{a: a, b: b, width: width}
}

type line2D struct {
	width float32
	a, b  ms2.Vec
}

func (l *line2D) Bounds() ms2.Box {
	w := l.width / 2
	b := ms2.Box{Min: l.a, Max: l.b}.Canon()
	b.Max = ms2.AddScalar(w, b.Max)
	b.Min = ms2.AddScalar(-w, b.Min)
	return b
}

func (l *line2D) AppendShaderName(b []byte) []byte {
	b = append(b, "line"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', l.a.X, l.a.Y, l.b.X, l.b.Y, l.width)
	return b
}

func (l *line2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2Decl(b, "a", l.a)
	b = glbuild.AppendVec2Decl(b, "ba", ms2.Sub(l.b, l.a))
	b = glbuild.AppendFloatDecl(b, "w", l.width/2)
	b = append(b, `vec2 pa=p-a;
float h=clamp( dot(pa,ba)/dot(ba,ba), 0.0, 1.0); 
return length(pa-ba*h)-w;`...)
	return b
}

func (l *line2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *line2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

// NewLines2D creates sequential straight lines between the argument points.
func (bld *Builder) NewLines2D(segments [][2]ms2.Vec, width float32) glbuild.Shader2D {
	if width < 0 {
		bld.shapeErrorf("negative thickness to NewLines2D")
	}
	if len(segments) < 2 {
		bld.shapeErrorf("empty or single points")
	}
	for _, v := range segments[:len(segments)-1] {
		if v[0] == v[1] {
			bld.shapeErrorf("superimposed points in NewLines2D")
		}
	}
	hash := hash2vec2(segments...) + width
	bufName := []byte("ssboLines2d_")
	bufName = glbuild.AppendFloat(bufName, 'n', 'p', hash)
	return &lines2D{points: segments, width: width, bufName: bufName, hash: hash}
}

type lines2D struct {
	hash    float32
	bufName []byte
	points  [][2]ms2.Vec
	width   float32
}

func (l *lines2D) Bounds() ms2.Box {
	w := l.width / 2
	bb := ms2.NewBox(l.points[0][0].X, l.points[0][0].Y, l.points[0][1].X, l.points[0][1].Y)
	for _, v := range l.points[1:] {
		bb = bb.IncludePoint(v[0])
		bb = bb.IncludePoint(v[1])
	}
	bb.Max = ms2.AddScalar(w, bb.Max)
	bb.Min = ms2.AddScalar(-w, bb.Min)
	return bb
}

func (l *lines2D) AppendShaderName(b []byte) []byte {
	b = append(b, "lines"...)
	b = glbuild.AppendFloat(b, 'n', 'p', l.hash)
	return b
}

func (l *lines2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "w", l.width/2)
	b = glbuild.AppendDefineDecl(b, "points", string(l.bufName))
	b = append(b, `const int num = points.length();
float d2 = 1.0e23;
for (int i=0; i<num; i++)
{
	vec4 v1v2 = points[i];
	vec2 a = v1v2.xy;
	vec2 b = v1v2.zw;
	vec2 pa = p-a, ba = b-a;
	float h = clamp( dot(pa,ba)/dot(ba,ba), 0.0, 1.0 );
	vec2 dv = pa -ba*h;
	d2 = min(d2, dot(dv,dv) );
}
return sqrt(d2)-w;
`...)
	b = glbuild.AppendUndefineDecl(b, "points")
	return b
}

func (l *lines2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *lines2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	ssbo, err := glbuild.MakeShaderBufferReadOnly(u.bufName, u.points)
	if err != nil {
		panic(err)
	}
	return append(objects, ssbo)
}

// NewArc returns a 2D arc centered at the origin (x,y)=(0,0) for a given radius and arc angle and thickness of the arc.
// The arc begins opening at (x,y)=(0,r) in both positive and negative x direction.
func (bld *Builder) NewArc(radius, arcAngle, thick float32) glbuild.Shader2D {
	ok := radius > 0 && arcAngle > 0 && thick >= 0
	if !ok {
		bld.shapeErrorf("invalid argument to NewArc2D")
	}
	if arcAngle > 2*math.Pi {
		bld.shapeErrorf("arc angle exceeds full circle")
	} else if 2*math.Pi-arcAngle < epstol {
		arcAngle = 2*math.Pi - 1e-7 // Condition the arc to be closed.
	}
	return &arc2D{radius: radius, angle: arcAngle, thick: thick}
}

type arc2D struct {
	radius float32
	angle  float32
	thick  float32
}

func (a *arc2D) Bounds() ms2.Box {
	r := a.radius + a.thick
	rcos := a.radius*math32.Cos(a.angle/2) - a.thick
	return ms2.Box{
		Min: ms2.Vec{X: -r, Y: rcos},
		Max: ms2.Vec{X: r, Y: r},
	}
}

func (a *arc2D) AppendShaderName(b []byte) []byte {
	b = append(b, "arc"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', a.radius, a.angle, a.thick)
	return b
}

func (a *arc2D) AppendShaderBody(b []byte) []byte {
	s, c := math32.Sincos(a.angle / 2)
	b = glbuild.AppendFloatDecl(b, "r", a.radius)
	b = glbuild.AppendFloatDecl(b, "t", a.thick/2)
	b = glbuild.AppendVec2Decl(b, "sc", ms2.Vec{X: s, Y: c})
	b = append(b, `p.x=abs(p.x);
return ((sc.y*p.x>sc.x*p.y) ? length(p-sc*r) : abs(length(p)-r))-t;`...)
	return b
}

func (a *arc2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *arc2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type circle2D struct {
	r float32
}

// NewCircle creates a circle of a radius centered at the origin (x,y)=(0,0).
func (bld *Builder) NewCircle(radius float32) glbuild.Shader2D {
	okRadius := radius > 0 && !math32.IsInf(radius, 1)
	if !okRadius {
		bld.shapeErrorf("bad circle radius: " + strconv.FormatFloat(float64(radius), 'g', 6, 32))
	}
	return &circle2D{r: radius}
}

func (c *circle2D) Bounds() ms2.Box {
	r := c.r
	return ms2.NewBox(-r, -r, r, r)
}

func (c *circle2D) AppendShaderName(b []byte) []byte {
	b = append(b, "circle"...)
	b = glbuild.AppendFloat(b, 'n', 'p', c.r)
	return b
}

func (c *circle2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "r", c.r)
	b = append(b, "return length(p)-r;"...)
	return b
}

func (c *circle2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *circle2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type equilateralTri2d struct {
	hTri float32
}

// NewEquilateralTriangle creates an equilater triangle with a given height with it's centroid located at the origin.
func (bld *Builder) NewEquilateralTriangle(triangleHeight float32) glbuild.Shader2D {
	okTri := triangleHeight > 0 && !math32.IsInf(triangleHeight, 1)
	if !okTri {
		bld.shapeErrorf("bad equilateral triangle height")
	}
	return &equilateralTri2d{hTri: triangleHeight}
}

func (t *equilateralTri2d) Bounds() ms2.Box {
	height := t.hTri
	side := height / tribisect
	longBisect := side / sqrt3    // (L/2)/cosd(30)
	shortBisect := longBisect / 2 // (L/2)/tand(60)
	return ms2.Box{
		Min: ms2.Vec{X: -side / 2, Y: -shortBisect},
		Max: ms2.Vec{X: side / 2, Y: longBisect},
	}
}

func (t *equilateralTri2d) AppendShaderName(b []byte) []byte {
	b = append(b, "circle"...)
	b = glbuild.AppendFloat(b, 'n', 'p', t.hTri)
	return b
}

func (t *equilateralTri2d) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "h", t.hTri/sqrt3)
	b = append(b, `const float k = sqrt(3.0);
    p.x = abs(p.x) - h;
    p.y = p.y + h/k;
    if( p.x+k*p.y>0.0 ) p = vec2(p.x-k*p.y,-k*p.x-p.y)/2.0;
    p.x -= clamp( p.x, -2.0*h, 0.0 );
    return -length(p)*sign(p.y);`...)
	return b
}

func (t *equilateralTri2d) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *equilateralTri2d) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type rect2D struct {
	d ms2.Vec
}

// NewRectangle creates a rectangle centered at (x,y)=(0,0) with given x and y dimensions.
func (bld *Builder) NewRectangle(x, y float32) glbuild.Shader2D {
	okRect := x > 0 && y > 0 && !math32.IsInf(x, 1) && !math32.IsInf(y, 1)
	if !okRect {
		bld.shapeErrorf("bad rectangle dimension")
	}
	return &rect2D{d: ms2.Vec{X: x, Y: y}}
}

func (c *rect2D) Bounds() ms2.Box {
	xd2 := c.d.X / 2
	yd2 := c.d.Y / 2
	return ms2.Box{
		Min: ms2.Vec{X: -xd2, Y: -yd2},
		Max: ms2.Vec{X: xd2, Y: yd2},
	}
}

func (c *rect2D) AppendShaderName(b []byte) []byte {
	b = append(b, "rect"...)
	arr := c.d.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	return b
}

func (c *rect2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2Decl(b, "b", ms2.Scale(0.5, c.d))
	b = append(b, `vec2 d = abs(p)-b;
    return length(max(d,0.0)) + min(max(d.x,d.y),0.0);`...)
	return b
}

func (c *rect2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *rect2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type hex2D struct {
	side float32
}

// NewHexagon creates a regular hexagon centered at (x,y)=(0,0) with sides of length `side`.
func (bld *Builder) NewHexagon(side float32) glbuild.Shader2D {
	okHex := side > 0 && !math32.IsInf(side, 1)
	if !okHex {
		bld.shapeErrorf("bad hexagon dimension")
	}
	return &hex2D{side: side}
}

func (c *hex2D) Bounds() ms2.Box {
	s := c.side
	return ms2.NewBox(-s, -s, s, s)
}

func (c *hex2D) AppendShaderName(b []byte) []byte {
	b = append(b, "hex2d"...)
	b = glbuild.AppendFloat(b, 'n', 'p', c.side)
	return b
}

func (c *hex2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "r", c.side)
	b = append(b, `const vec3 k = vec3(-0.8660254038,0.5,0.577350269);
p = abs(p);
p -= 2.0*min(dot(k.xy,p),0.0)*k.xy;
p -= vec2(clamp(p.x, -k.z*r, k.z*r), r);
return length(p)*sign(p.y);`...)
	return b
}

func (c *hex2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *hex2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type ellipse2D struct {
	a, b float32
}

// NewEllipse creates a 2D ellipse SDF with a and b ellipse parameters.
func (bld *Builder) NewEllipse(a, b float32) glbuild.Shader2D {
	okEllipse := a > 0 && b > 0 && !math32.IsInf(a, 1) && !math32.IsInf(b, 1)
	if !okEllipse {
		bld.shapeErrorf("bad ellipse dimension")
	}
	return &ellipse2D{a: a, b: b}
}

func (c *ellipse2D) Bounds() ms2.Box {
	a := c.a
	b := c.b
	return ms2.NewBox(-a, -b, a, b)
}

func (c *ellipse2D) AppendShaderName(b []byte) []byte {
	b = append(b, "ellipse2D"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', c.a, c.b)
	return b
}

func (c *ellipse2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2Decl(b, "ab", ms2.Vec{X: c.a, Y: c.b})
	b = append(b, `p = abs(p);
if( p.x > p.y ) {
	p=p.yx;
	ab=ab.yx;
}
float l = ab.y*ab.y - ab.x*ab.x;
float m = ab.x*p.x/l;
float m2 = m*m;
float n = ab.y*p.y/l;
float n2 = n*n; 
float c = (m2+n2-1.0)/3.0;
float c3 = c*c*c;
float q = c3 + m2*n2*2.0;
float d = c3 + m2*n2;
float g = m + m*n2;
float co;
if ( d<0.0 ) {
	float h = acos(q/c3)/3.0;
	float s = cos(h);
	float t = sin(h)*sqrt(3.0);
	float rx = sqrt( -c*(s + t + 2.0) + m2 );
	float ry = sqrt( -c*(s - t + 2.0) + m2 );
	co = (ry+sign(l)*rx+abs(g)/(rx*ry)- m)/2.0;
} else {
	float h = 2.0*m*n*sqrt( d );
	float s = sign(q+h)*pow(abs(q+h), 1.0/3.0);
	float u = sign(q-h)*pow(abs(q-h), 1.0/3.0);
	float rx = -s - u - c*4.0 + 2.0*m2;
	float ry = (s - u)*sqrt(3.0);
	float rm = sqrt( rx*rx + ry*ry );
	co = (ry/sqrt(rm-rx)+2.0*g/rm-m)/2.0;
}
vec2 r = ab * vec2(co, sqrt(1.0-co*co));
return length(r-p) * sign(p.y-r.y);`...)
	return b
}

func (c *ellipse2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *ellipse2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type poly2D struct {
	vert []ms2.Vec
}

// NewPolygon creates a polygon from a set of vertices. The polygon can be self-intersecting.
func (bld *Builder) NewPolygon(vertices []ms2.Vec) glbuild.Shader2D {
	prevIdx := len(vertices) - 1
	if vertices[0] == vertices[prevIdx] {
		vertices = vertices[:prevIdx] // Discard last vertex if equal to first (this algorithm closes automatically).
		prevIdx--
	}
	if len(vertices) < 3 {
		bld.shapeErrorf("polygon needs at least 3 distinct vertices")
	}
	for i := range vertices {
		if math32.IsNaN(vertices[i].X) || math32.IsNaN(vertices[i].Y) {
			bld.shapeErrorf("NaN value in vertices")
		}
		if vertices[i] == vertices[prevIdx] {
			bld.shapeErrorf("found two consecutive equal vertices in polygon")
		}
		prevIdx = i
	}
	return &poly2D{vert: vertices}
}

func (c *poly2D) Bounds() ms2.Box {
	min := ms2.Vec{X: largenum, Y: largenum}
	max := ms2.Vec{X: -largenum, Y: -largenum}
	for _, v := range c.vert {
		min = ms2.MinElem(min, v)
		max = ms2.MaxElem(max, v)
	}
	return ms2.Box{Min: min, Max: max}
}

func (c *poly2D) AppendShaderName(b []byte) []byte {
	var hash uint64 = 0xfafa0fa_c0feebeef
	for _, v := range c.vert {
		hash ^= uint64(math.Float32bits(v.X))
		hash ^= uint64(math.Float32bits(v.Y)) << 32
	}
	b = append(b, "poly2D"...)
	b = strconv.AppendUint(b, hash, 32)
	return b
}

func (c *poly2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2SliceDecl(b, "v", c.vert)
	b = append(b, `const int num = v.length();
float d = dot(p-v[0],p-v[0]);
float s = 1.0;
for( int i=0, j=num-1; i<num; j=i, i++ )
{
	// distance
	vec2 e = v[j] - v[i];
	vec2 w = p - v[i];
	vec2 b = w - e*clamp( dot(w,e)/dot(e,e), 0.0, 1.0 );
	d = min( d, dot(b,b) );
	// winding number from http://geomalgorithms.com/a03-_inclusion.html
	bvec3 cond = bvec3( p.y>=v[i].y, 
						p.y <v[j].y, 
						e.x*w.y>e.y*w.x );
	if( all(cond) || all(not(cond)) ) s=-s;  
}
return s*sqrt(d);`...)
	return b
}

func (c *poly2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *poly2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects // TODO: implement shader buffer storage here!
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
	b = append(b, `float pangle=atan(p.y, p.x);
	float i=floor(pangle/angle);
	if (i<0.0) i=ncirc+i;
	float i0,i1;
	if (i>=ninsm1) {
		i0=ninsm1;
		i1=0.0;
	} else {
		i0=i;
		i1=i+1.0;
	}
	float c0 = cos(angle*i0);
	float s0 = sin(angle*i0);
	vec2 p0 = mat2(c0,-s0,s0,c0)*p;
	float c1 = cos(angle*i1);
	float s1 = sin(angle*i1);
	vec2 p1 = mat2(c1,-s1,s1,c1)*p;
	`...)
	b = glbuild.AppendDistanceDecl(b, "d0", "p0", ca.s)
	b = glbuild.AppendDistanceDecl(b, "d1", "p1", ca.s)
	b = append(b, "return min(d0, d1);"...)
	return b
}

func (u *circarray2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
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
