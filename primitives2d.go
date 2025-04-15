package gsdf

import (
	"errors"
	"math"
	"strconv"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/gsdf/glbuild"
)

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

type oct2D struct {
	c float32
}

// NewOctagon returns a regular octagon 2D SDF with form that extend up to -constrain and constrain in both x and y axes.
func (bld *Builder) NewOctagon(constrain float32) glbuild.Shader2D {
	okOct := constrain > 0
	if !okOct {
		bld.shapeErrorf("bad octagon dimension %f", constrain)
	}
	return &oct2D{c: constrain}
}

func (oct *oct2D) Bounds() ms2.Box {
	s := oct.c
	return ms2.NewBox(-s, -s, s, s)
}

func (oct *oct2D) AppendShaderName(b []byte) []byte {
	b = append(b, "oct2D"...)
	b = glbuild.AppendFloat(b, 'n', 'p', oct.c)
	return b
}

func (oct *oct2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "r", oct.c)
	b = append(b, `const vec3 k = vec3(-0.9238795325, 0.3826834323, 0.4142135623 );
	p = abs(p);
	p -= 2.0*min(dot(vec2( k.x,k.y),p),0.0)*vec2( k.x,k.y);
	p -= 2.0*min(dot(vec2(-k.x,k.y),p),0.0)*vec2(-k.x,k.y);
	p -= vec2(clamp(p.x, -k.z*r, k.z*r), r);
	return length(p)*sign(p.y);`...)
	return b
}

func (oct *oct2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (oct *oct2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type ellipse2D struct {
	a, b float32
}

// NewEllipse creates a 2D ellipse SDF with a and b ellipse parameters.
func (bld *Builder) NewEllipse(a, b float32) glbuild.Shader2D {
	okEllipse := a > 0 && b > 0 && !math32.IsInf(a, 1) && !math32.IsInf(b, 1)
	if !okEllipse {
		bld.shapeErrorf("bad ellipse dimension (a=%f, b=%f)", a, b)
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
	vertices, err := bld.validatePolygon(vertices)
	if err != nil {
		bld.shapeErrorf(err.Error())
	}
	poly := poly2D{vert: vertices}
	if bld.useGPU(len(vertices) * 2) {
		return &polyGPU{poly2D: poly, bufname: makeHashName(nil, "ssboPoly", vertices)}
	}
	return &poly
}

func (bld *Builder) validatePolygon(vertices []ms2.Vec) ([]ms2.Vec, error) {
	prevIdx := len(vertices) - 1
	if vertices[0] == vertices[prevIdx] {
		vertices = vertices[:prevIdx] // Discard last vertex if equal to first (this algorithm closes automatically).
		prevIdx--
	}
	if len(vertices) < 3 {
		return vertices, errors.New("polygon needs at least 3 distinct vertices")
	}
	for i := range vertices {
		if math32.IsNaN(vertices[i].X) || math32.IsNaN(vertices[i].Y) {
			return vertices, errors.New("NaN value in vertices")
		}
		if vertices[i] == vertices[prevIdx] {
			return vertices, errors.New("found two consecutive equal vertices in polygon")
		}
		prevIdx = i
	}
	return vertices, nil
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

const polyShader = `const int num = v.length();
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
return s*sqrt(d);
`

func (c *poly2D) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2SliceDecl(b, "v", c.vert)
	b = append(b, polyShader...)
	return b
}

func (c *poly2D) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *poly2D) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects // TODO: implement shader buffer storage here!
}

type polyGPU struct {
	poly2D
	bufname []byte
}

func (c *polyGPU) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendDefineDecl(b, "v", string(c.bufname))
	b = append(b, polyShader...)
	b = glbuild.AppendUndefineDecl(b, "v")
	return b
}

func (u *polyGPU) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	ssbo, err := glbuild.MakeShaderBufferReadOnly(u.bufname, u.vert)
	if err != nil {
		panic(err)
	}
	return append(objects, ssbo)
}

type diamond struct {
	d ms2.Vec
}

// NewDiamond2D creates a diamond (rhombus) centered at (x,y)=(0,0) with given x and y width and height dimensions, respectively.
func (bld *Builder) NewDiamond2D(x_width, y_height float32) glbuild.Shader2D {
	okRect := x_width > 0 && y_height > 0 && !math32.IsInf(x_width, 1) && !math32.IsInf(y_height, 1)
	if !okRect {
		bld.shapeErrorf("bad diamond dimension")
	}
	return &diamond{d: ms2.Vec{X: x_width, Y: y_height}}
}

func (c *diamond) Bounds() ms2.Box {
	xd2 := c.d.X / 2
	yd2 := c.d.Y / 2
	return ms2.Box{
		Min: ms2.Vec{X: -xd2, Y: -yd2},
		Max: ms2.Vec{X: xd2, Y: yd2},
	}
}

func (c *diamond) AppendShaderName(b []byte) []byte {
	b = append(b, "diamond"...)
	arr := c.d.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	return b
}

func (c *diamond) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2Decl(b, "b", ms2.Scale(0.5, c.d))
	b = append(b, `p = abs(p);
	float ndot = b.x*(b.x-2.*p.x)-b.y*(b.y-2*p.y);
	float h = clamp( ndot/dot(b,b), -1.0, 1.0 );
	float d = length( p-0.5*b*vec2(1.0-h,1.0+h) );
	return d * sign( p.x*b.y + p.y*b.x - b.x*b.y );`...)
	return b
}

func (c *diamond) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *diamond) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type x2d struct {
	dim   float32
	thick float32
}

// NewRoundedX creates a two-dimensional X centered at (x,y)=(0,0) with given width and thickness/radius of lines.
func (bld *Builder) NewRoundedX(width, thick float32) glbuild.Shader2D {
	okRect := width > 0 && thick > 0 && !math32.IsInf(width, 1) && !math32.IsInf(thick, 1)
	if !okRect {
		bld.shapeErrorf("bad x dimension")
	}
	return &x2d{dim: width, thick: thick}
}

func (c *x2d) Bounds() ms2.Box {
	xd2 := c.dim/2 + c.thick
	return ms2.Box{
		Min: ms2.Vec{X: -xd2, Y: -xd2},
		Max: ms2.Vec{X: xd2, Y: xd2},
	}
}

func (c *x2d) AppendShaderName(b []byte) []byte {
	b = append(b, "x2d"...)
	b = glbuild.AppendFloats(b, 0, 'n', 'p', c.dim, c.thick)
	return b
}

func (c *x2d) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendFloatDecl(b, "w", c.dim)
	b = glbuild.AppendFloatDecl(b, "r", c.thick)
	b = append(b, `p = abs(p);
	return length(p-min(p.x+p.y,w)*0.5) - r;`...)
	return b
}

func (c *x2d) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *x2d) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}

type quadbezier2d struct {
	a, b, c ms2.Vec // Control points of quadratic bezier.
	thick   float32
}

// NewQuadraticBezier2D creats an exact quadratic bezier SDF with three control points a, b, c. a and c lie on the bezier, b is a slope control point.
// Thick is the thickness of the curve.
func (bld *Builder) NewQuadraticBezier2D(a, b, c ms2.Vec, thick float32) glbuild.Shader2D {
	return &quadbezier2d{a: a, b: b, c: c, thick: thick}
}

func (c *quadbezier2d) Bounds() ms2.Box {
	min := ms2.AddScalar(-c.thick/2, ms2.MinElem(c.a, ms2.MinElem(c.b, c.c)))
	max := ms2.AddScalar(c.thick/2, ms2.MaxElem(c.a, ms2.MaxElem(c.b, c.c)))
	return ms2.Box{
		Min: min,
		Max: max,
	}
}

func (c *quadbezier2d) AppendShaderName(b []byte) []byte {
	b = append(b, "quadbezier2d"...)
	arr := c.a.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	arr = c.b.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	arr = c.c.Array()
	b = glbuild.AppendFloats(b, 0, 'n', 'p', arr[:]...)
	b = glbuild.AppendFloat(b, 'n', 'p', c.thick)
	return b
}

func (c *quadbezier2d) AppendShaderBody(b []byte) []byte {
	b = glbuild.AppendVec2Decl(b, "A", c.a)
	b = glbuild.AppendVec2Decl(b, "B", c.b)
	b = glbuild.AppendVec2Decl(b, "C", c.c)
	b = glbuild.AppendFloatDecl(b, "thick", c.thick/2)
	b = append(b, `vec2 a = B - A;
	vec2 b = A + C - 2.0*B;
	vec2 c = a * 2.0;
	vec2 d = A - p;
	float kk = 1.0/dot(b,b);
	float kx = kk * dot(a,b);
	float ky = kk * (2.0*dot(a,a)+dot(d,b))/3.0;
	float kz = kk * dot(d,a);
	float res = 0.0;
	float sgn = 0.0;
	float g  = ky - kx*kx;
	float q  = kx*(2.0*kx*kx - 3.0*ky) + kz;
	float g3 = g*g*g;
	float q2 = q*q;
	float h  = q2 + 4.0*g3;
	if ( h>=0.0 ) 
	{
		h = sqrt(h);
		vec2 x = (vec2(h,-h)-q)/2.0;
		if ( abs(g)<0.001 ) 
		{
			float k = (1.0-g3/q2)*g3/q;
			x = vec2(k,-k-q);
		}
		vec2 uv = sign(x)*pow(abs(x), vec2(1.0/3.0));
		float t = uv.x + uv.y;
		t -= (t*(t*t+3.0*g)+q)/(3.0*t*t+3.0*g);
		t = clamp( t-kx, 0.0, 1.0 );
		vec2  w = d+(c+b*t)*t;
		res = dot(w,w);
		vec2 aux = c+2.0*b*t;
		sgn = aux.x*w.y-aux.y*w.x;
	} else {
		float z = sqrt(-g);
		float aux = q/(g*z*2.0);
		float x = sqrt(0.5+0.5*aux);
		float m = x*(x*(x*(x*-0.008972+0.039071)-0.107074)+0.576975)+0.5;
		float n = sqrt(1.0-m*m);
		n *= sqrt(3.0);
		vec3  t = clamp( vec3(m+m,-n-m,n-m)*z-kx, 0.0, 1.0 );
		vec2 aux2 = a+b*t.x;
		vec2  qx=d+(c+b*t.x)*t.x; float dx=dot(qx,qx), sx=aux2.x*qx.y - aux2.y*qx.x;
		aux2 = a+b*t.y;
		vec2  qy=d+(c+b*t.y)*t.y; float dy=dot(qy,qy), sy=aux2.x*qy.y - aux2.y*qy.x;
		if( dx<dy ) {res=dx;sgn=sx;} else {res=dy;sgn=sy;}
	}
	return sqrt( res ) - thick;`...)
	return b
}

func (c *quadbezier2d) ForEach2DChild(userData any, fn func(userData any, s *glbuild.Shader2D) error) error {
	return nil
}

func (u *quadbezier2d) AppendShaderObjects(objects []glbuild.ShaderObject) []glbuild.ShaderObject {
	return objects
}
