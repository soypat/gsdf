package glbuild

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
)

//go:embed visualizer_footer.tmpl
var visualizerFooter []byte

// Shader stores information for automatically generating SDF Shader pipelines.
type Shader interface {
	// AppendShaderName appends the name of the GL shader function
	// to the buffer and returns the result. It should be unique to that shader.
	AppendShaderName(b []byte) []byte
	// AppendShaderBody appends the body of the shader function to the
	// buffer and returns the result.
	AppendShaderBody(b []byte) []byte
}

// Shader3D can create SDF shader source code for an arbitrary 3D shape.
type Shader3D interface {
	Shader
	// ForEachChild iterats over the Shader3D's direct Shader3D children.
	// Unary operations have one child i.e: Translate, Transform, Scale.
	// Binary operations have two children i.e: Union, Intersection, Difference.
	ForEachChild(userData any, fn func(userData any, s *Shader3D) error) error
	// Bounds returns the Shader3D's bounding box where the SDF is negative.
	Bounds() ms3.Box
}

// Shader2D can create SDF shader source code for an arbitrary 2D shape.
type Shader2D interface {
	Shader
	// ForEachChild iterats over the Shader2D's direct Shader2D children.
	// Unary operations have one child i.e: Translate, Scale.
	// Binary operations have two children i.e: Union, Intersection, Difference.
	ForEach2DChild(userData any, fn func(userData any, s *Shader2D) error) error
	// Bounds returns the Shader2D's bounding box where the SDF is negative.
	Bounds() ms2.Box
}

// shader3D2D can create SDF shader source code for a operation that receives 2D
// shaders to generate a 3D shape.
type shader3D2D interface {
	Shader3D
	ForEach2DChild(userData any, fn func(userData any, s *Shader2D) error) error
}

// Programmer implements shader generation logic for Shader type.
type Programmer struct {
	scratchNodes  []Shader
	scratch       []byte
	computeHeader []byte
}

var defaultComputeHeader = []byte("#shader compute\n#version 430\n")

// NewDefaultProgrammer returns a Programmer with reasonable default parameters for use with glgl package.
func NewDefaultProgrammer() *Programmer {
	return &Programmer{
		scratchNodes:  make([]Shader, 64),
		scratch:       make([]byte, 1024),
		computeHeader: defaultComputeHeader,
	}
}

// WriteDistanceIO creates the bare bones I/O compute program for calculating SDF
// and writes it to the writer.
func (p *Programmer) WriteComputeSDF3(w io.Writer, obj Shader) (int, error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], obj)
	if err != nil {
		return 0, err
	}
	// Begin writing shader source code.
	n, err := w.Write(p.computeHeader)
	if err != nil {
		return n, err
	}
	ngot, newScratch, err := WriteShaders(w, nodes, p.scratch)
	p.scratch = newScratch
	n += ngot
	if err != nil {
		return n, err
	}

	ngot, err = fmt.Fprintf(w, `

layout(local_size_x = 1, local_size_y = 1, local_size_z = 1) in;
layout(rgba32f, binding = 0) uniform image2D in_tex;
// The binding argument refers to the textures Unit.
layout(r32f, binding = 1) uniform image2D out_tex;

void main() {
	// get position to read/write data from.
	ivec2 pos = ivec2( gl_GlobalInvocationID.xy );
	// Get SDF position value.
	vec3 p = imageLoad( in_tex, pos ).rgb;
	float distance = %s(p);
	// store new value in image
	imageStore( out_tex, pos, vec4( distance, 0.0, 0.0, 0.0 ) );
}
`, baseName)

	n += ngot
	return n, err
}

// WriteFragVisualizerSDF3 generates a OpenGL program that can be visualized in most shader visualizers such as ShaderToy.
func (p *Programmer) WriteFragVisualizerSDF3(w io.Writer, obj Shader3D) (n int, err error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], obj)
	if err != nil {
		return 0, err
	}
	ngot, newScratch, err := WriteShaders(w, nodes, p.scratch)
	p.scratch = newScratch
	n += ngot
	if err != nil {
		return n, err
	}
	ngot, err = w.Write([]byte("\nfloat sdf(vec3 p) { return " + baseName + "(p); }\n\n"))
	n += ngot
	if err != nil {
		return n, err
	}
	ngot, err = w.Write(visualizerFooter)
	n += ngot
	if err != nil {
		return n, err
	}
	return n, nil
}

// ParseAppendNodes parses the shader object tree and appends all nodes in Depth First order
// to the dst Shader argument buffer and returns the result.
func ParseAppendNodes(dst []Shader, root Shader) (baseName string, nodes []Shader, err error) {
	if root == nil {
		return "", nil, errors.New("nil shader object")
	}
	baseName = string(root.AppendShaderName([]byte{}))
	if baseName == "" {
		return "", nil, errors.New("empty shader name")
	}
	dst, err = AppendAllNodes(dst, root)
	if err != nil {
		return "", nil, err
	}
	return baseName, dst, nil
}

// WriteShaders iterates over the argument nodes in reverse order and
// writes their GL code to the writer. scratch is an auxiliary buffer to avoid heap allocations.
//
// WriteShaders does not check for
func WriteShaders(w io.Writer, nodes []Shader, scratch []byte) (n int, newscratch []byte, err error) {
	if scratch == nil {
		scratch = make([]byte, 512)
	}
	var ngot int
	for i := len(nodes) - 1; i >= 0; i-- {
		ngot, scratch, err = WriteShader(w, nodes[i], scratch[:0])
		n += ngot
		if err != nil {
			return n, scratch, err
		}
	}
	return n, scratch, nil
}

// WriteShader writes the GL code of a single shader to the writer. scratch is an auxiliary buffer to prevent allocations. If scratch's
// capacity is grown during the writing the buffer with augmented capacity is returned. If not the same input scratch is returned.
func WriteShader(w io.Writer, s Shader, scratch []byte) (int, []byte, error) {
	scratch = scratch[:0]
	scratch = append(scratch, "float "...)
	scratch = s.AppendShaderName(scratch)
	if _, ok := s.(Shader3D); ok {
		scratch = append(scratch, "(vec3 p) {\n"...)
	} else {
		scratch = append(scratch, "(vec2 p) {\n"...)
	}
	scratch = s.AppendShaderBody(scratch)
	scratch = append(scratch, "\n}\n\n"...)
	n, err := w.Write(scratch)
	return n, scratch, err
}

// AppendAllNodes BFS iterates over all of root's descendants and appends all nodes
// found to dst.
//
// To generate shaders one must iterate over nodes in reverse order to ensure
// the first iterated nodes are the nodes with no dependencies on other nodes.
func AppendAllNodes(dst []Shader, root Shader) ([]Shader, error) {
	var userData any
	children := []Shader{root}
	nextChild := 0
	nilChild := errors.New("got nil child in AppendAllNodes")
	for len(children[nextChild:]) > 0 {
		newChildren := children[nextChild:]
		for _, obj := range newChildren {
			nextChild++
			obj3, ok3 := obj.(Shader3D)
			obj2, ok2 := obj.(Shader2D)
			if !ok2 && !ok3 {
				return nil, fmt.Errorf("found shader %T that does not implement Shader3D nor Shader2D", obj)
			}
			var err error
			if ok3 {
				// Got Shader3D in obj.
				err = obj3.ForEachChild(userData, func(userData any, s *Shader3D) error {
					if s == nil || *s == nil {
						return nilChild
					}
					children = append(children, *s)
					return nil
				})
				if obj32, ok32 := obj.(shader3D2D); ok32 {
					// The Shader3D obj contains Shader2D children, such is case for 2D->3D operations i.e: revolution and extrusion operations.
					err = obj32.ForEach2DChild(userData, func(userData any, s *Shader2D) error {
						if s == nil || *s == nil {
							return nilChild
						}
						children = append(children, *s)
						return nil
					})
				}
			}
			if err == nil && !ok3 && ok2 {
				// Got Shader2D in obj.
				err = obj2.ForEach2DChild(userData, func(userData any, s *Shader2D) error {
					if s == nil || *s == nil {
						return nilChild
					}
					children = append(children, *s)
					return nil
				})
			}
			if err != nil {
				return nil, err
			}
		}
	}
	dst = append(dst, children...)
	return dst, nil
}

func AppendDistanceDecl(b []byte, s Shader, name, input string) []byte {
	b = append(b, "float "...)
	b = append(b, name...)
	b = append(b, '=')
	b = s.AppendShaderName(b)
	b = append(b, '(')
	b = append(b, input...)
	b = append(b, ");\n"...)
	return b
}

func AppendVec3Decl(b []byte, name string, v ms3.Vec) []byte {
	b = append(b, "vec3 "...)
	b = append(b, name...)
	b = append(b, "=vec3("...)
	arr := v.Array()
	b = AppendFloats(b, arr[:], ',', '-', '.')
	b = append(b, ')', ';', '\n')
	return b
}

func AppendVec2Decl(b []byte, name string, v ms2.Vec) []byte {
	b = append(b, "vec2 "...)
	b = append(b, name...)
	b = append(b, "=vec2("...)
	arr := v.Array()
	b = AppendFloats(b, arr[:], ',', '-', '.')
	b = append(b, ')', ';', '\n')
	return b
}

func AppendFloatDecl(b []byte, name string, v float32) []byte {
	b = append(b, "float "...)
	b = append(b, name...)
	b = append(b, '=')
	b = AppendFloat(b, v, '-', '.')
	b = append(b, ';', '\n')
	return b
}

func AppendMat4Decl(b []byte, name string, m44 ms3.Mat4) []byte {
	arr := m44.Array()
	b = append(b, "mat4 "...)
	b = append(b, name...)
	b = append(b, "=mat4("...)
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			v := arr[j*4+i] // Column major access, as per OpenGL standard.
			b = AppendFloat(b, v, '-', '.')
			last := i == 3 && j == 3
			if !last {
				b = append(b, ',')
			}
		}
	}
	b = append(b, ");\n"...)
	return b
}

func AppendFloat(b []byte, v float32, neg, decimal byte) []byte {
	start := len(b)
	b = strconv.AppendFloat(b, float64(v), 'f', 6, 32)
	idx := bytes.IndexByte(b[start:], '.')
	if decimal != '.' && idx >= 0 {
		b[start+idx] = decimal
	}
	if b[start] == '-' {
		b[start] = neg
	}
	// Finally trim zeroes.
	end := len(b)
	for i := len(b) - 1; idx >= 0 && i > idx+start && b[i] == '0'; i-- {
		end--
	}
	return b[:end]
}

func AppendFloats(b []byte, s []float32, sep, neg, decimal byte) []byte {
	for i, v := range s {
		b = AppendFloat(b, v, neg, decimal)
		if sep != 0 && i != len(s)-1 {
			b = append(b, sep)
		}
	}
	return b
}

type XYZBits uint8

const (
	xBit XYZBits = 1 << iota
	yBit
	zBit
)

func (xyz XYZBits) X() bool { return xyz&xBit != 0 }
func (xyz XYZBits) Y() bool { return xyz&yBit != 0 }
func (xyz XYZBits) Z() bool { return xyz&zBit != 0 }

func NewXYZBits(x, y, z bool) XYZBits {
	return XYZBits(b2i(x) | b2i(y)<<1 | b2i(z)<<2)
}

func (xyz XYZBits) AppendMapped(b []byte, Map [3]byte) []byte {
	if xyz.X() {
		b = append(b, Map[0])
	}
	if xyz.Y() {
		b = append(b, Map[1])
	}
	if xyz.Z() {
		b = append(b, Map[2])
	}
	return b
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

var _ Shader3D = (*CachedShader3D)(nil) // Interface implementation compile-time check.

// CachedShader3D implements the Shader3D interface with results it caches for another Shader3D on a call to RefreshCache.
type CachedShader3D struct {
	Shader     Shader3D
	bb         ms3.Box
	data       []byte
	bodyOffset int
}

// RefreshCache updates the cache with current values of the underlying shader.
func (c3 *CachedShader3D) RefreshCache() {
	c3.bb = c3.Shader.Bounds()
	c3.data = c3.Shader.AppendShaderName(c3.data[:0])
	c3.bodyOffset = len(c3.data)
	c3.data = c3.Shader.AppendShaderBody(c3.data)
}

// Bounds returns the cached 3D bounds. Implements [Shader3D]. Update by calling RefreshCache.
func (c3 *CachedShader3D) Bounds() ms3.Box { return c3.bb }

// ForEachChild calls the underlying Shader's ForEachChild. Implements [Shader3D].
func (c3 *CachedShader3D) ForEachChild(userData any, fn func(userData any, s *Shader3D) error) error {
	return c3.Shader.ForEachChild(userData, fn)
}

// AppendShaderName returns the cached Shader name. Implements [Shader]. Update by calling RefreshCache.
func (c3 *CachedShader3D) AppendShaderName(b []byte) []byte {
	return append(b, c3.data[:c3.bodyOffset]...)
}

// AppendShaderBody returns the cached Shader function body. Implements [Shader]. Update by calling RefreshCache.
func (c3 *CachedShader3D) AppendShaderBody(b []byte) []byte {
	return append(b, c3.data[c3.bodyOffset:]...)
}

// ForEach2DChild calls the underlying Shader's ForEach2DChild. This method is called for 3D shapes that
// use 2D shaders such as extrude and revolution. Implements [Shader2D].
func (c3 *CachedShader3D) ForEach2DChild(userData any, fn func(userData any, s *Shader2D) error) (err error) {
	s2, ok := c3.Shader.(shader3D2D)
	if ok {
		err = s2.ForEach2DChild(userData, fn)
	}
	return err
}

var _ Shader2D = (*CachedShader2D)(nil) // Interface implementation compile-time check.

// CachedShader2D implements the Shader2D interface with results it caches for another Shader2D on a call to RefreshCache.
type CachedShader2D struct {
	Shader     Shader2D
	bb         ms2.Box
	data       []byte
	bodyOffset int
}

// RefreshCache updates the cache with current values of the underlying shader.
func (c2 *CachedShader2D) RefreshCache() {
	c2.bb = c2.Shader.Bounds()
	c2.data = c2.Shader.AppendShaderName(c2.data[:0])
	c2.bodyOffset = len(c2.data)
	c2.data = c2.Shader.AppendShaderBody(c2.data)
}

// Bounds returns the cached 2D bounds. Implements [Shader3D]. Update by calling RefreshCache.
func (c2 *CachedShader2D) Bounds() ms2.Box { return c2.bb }

// ForEachChild calls the underlying Shader's ForEachChild. Implements [Shader3D].
func (c2 *CachedShader2D) ForEach2DChild(userData any, fn func(userData any, s *Shader2D) error) error {
	return c2.Shader.ForEach2DChild(userData, fn)
}

// AppendShaderName returns the cached Shader name. Implements [Shader]. Update by calling RefreshCache.
func (c2 *CachedShader2D) AppendShaderName(b []byte) []byte {
	return append(b, c2.data[:c2.bodyOffset]...)
}

// AppendShaderBody returns the cached Shader function body. Implements [Shader]. Update by calling RefreshCache.
func (c2 *CachedShader2D) AppendShaderBody(b []byte) []byte {
	return append(b, c2.data[c2.bodyOffset:]...)
}
