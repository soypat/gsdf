package glbuild

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unsafe"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
)

const decimalDigits = 9

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
	// names maps shader names to body hashes for checking duplicates.
	names map[uint64]uint64
	// Invocations size in X (local group size) to give each compute work group.
	invocX int
}

var defaultComputeHeader = []byte("#shader compute\n#version 430\n")

// NewDefaultProgrammer returns a Programmer with reasonable default parameters for use with glgl package on the local machine.
func NewDefaultProgrammer() *Programmer {
	return &Programmer{
		scratchNodes:  make([]Shader, 64),
		scratch:       make([]byte, 1024), // Max length of shader token is around 1024..1060 characters.
		computeHeader: defaultComputeHeader,
		names:         make(map[uint64]uint64),
		invocX:        32,
	}
}

// SetComputeInvocations sets the work group local-sizes. x*y*z must be less than maximum number of invocations.
func (p *Programmer) SetComputeInvocations(x, y, z int) {
	if y != 1 || z != 1 {
		panic("unsupported")
	} else if x < 1 {
		panic("zero or negative X invocation size")
	}
	p.invocX = x
}

// ComputeInvocations returns the worker group invocation size in x y and z.
func (p *Programmer) ComputeInvocations() (int, int, int) {
	return p.invocX, 1, 1
}

// WriteDistanceIO creates the bare bones I/O compute program for calculating SDF
// and writes it to the writer.
func (p *Programmer) WriteComputeSDF3(w io.Writer, obj Shader3D) (int, error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], obj)
	if err != nil {
		return 0, err
	}
	// Begin writing shader source code.
	n, err := w.Write(p.computeHeader)
	if err != nil {
		return n, err
	}
	ngot, err := p.writeShaders(w, nodes)
	n += ngot
	if err != nil {
		return n, err
	}
	const sz = unsafe.Sizeof(ms3.Vec{})
	ngot, err = fmt.Fprintf(w, `

layout(local_size_x = %d, local_size_y = 1, local_size_z = 1) in;

// Input: 3D positions at which to evaluate SDF.
layout(std140, binding = 0) buffer PositionsBuffer {
    vec3 vbo_positions[];
};

// Output: Result of SDF evaluation are the distances. Maps to position buffer.
layout(std430, binding = 1) buffer DistancesBuffer {
    float vbo_distances[];
};

void main() {
	int idx = int( gl_GlobalInvocationID.x );
	
	vec3 p = vbo_positions[idx];    // Get position to evaluate SDF at.
	vbo_distances[idx] = %s(p);     // Evaluate SDF and store to distance buffer.
}
`, p.invocX, baseName)

	n += ngot
	return n, err
}

// WriteDistanceIO creates the bare bones I/O compute program for calculating SDF
// and writes it to the writer.
func (p *Programmer) WriteComputeSDF2(w io.Writer, obj Shader2D) (int, error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], obj)
	if err != nil {
		return 0, err
	}
	// Begin writing shader source code.
	n, err := w.Write(p.computeHeader)
	if err != nil {
		return n, err
	}
	ngot, err := p.writeShaders(w, nodes)
	n += ngot
	if err != nil {
		return n, err
	}
	ngot, err = fmt.Fprintf(w, `

layout(local_size_x = %d, local_size_y = 1, local_size_z = 1) in;

// Input: 2D positions at which to evaluate SDF.
layout(std430, binding = 0) buffer PositionsBuffer {
    vec2 vbo_positions[];
};

// Output: Result of SDF evaluation are the distances. Maps to position buffer.
layout(std430, binding = 1) buffer DistancesBuffer {
    float vbo_distances[];
};

void main() {
	int idx = int( gl_GlobalInvocationID.x );
	
	vec2 p = vbo_positions[idx];    // Get position to evaluate SDF at.
	vbo_distances[idx] = %s(p);     // Evaluate SDF and store to distance buffer.
}
`, p.invocX, baseName)

	n += ngot
	return n, err
}

// WriteFragVisualizerSDF3 generates a OpenGL program that can be visualized in most shader visualizers such as ShaderToy.
func (p *Programmer) WriteFragVisualizerSDF3(w io.Writer, obj Shader3D) (n int, err error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], obj)
	if err != nil {
		return 0, err
	}
	ngot, err := p.writeShaders(w, nodes)
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

func (p *Programmer) writeShaders(w io.Writer, nodes []Shader) (n int, err error) {
	clear(p.names)
	for i := len(nodes) - 1; i >= 0; i-- {
		node := nodes[i]
		var name, body []byte
		p.scratch, name, body = AppendShaderSource(p.scratch[:0], node)
		nameHash := hash(name, 0)
		bodyHash := hash(body, nameHash) // Body hash mixes name as well.
		gotBodyHash, nameConflict := p.names[nameHash]
		if nameConflict {
			// Name already exists in tree, check if bodies are identical.
			if bodyHash == gotBodyHash {
				continue // Shader already written and is identical, skip.
			}
			return n, fmt.Errorf("duplicate %T shader name %q w/ body:\n%s", node, name, body)
		} else {
			p.names[nameHash] = bodyHash // Not found, add it.
		}
		ngot, err := w.Write(p.scratch)
		n += ngot
		if err != nil {
			return n, err
		}
	}
	return n, err
}

const shorteningBufsize = 1024

func ShortenNames3D(root *Shader3D, maxRewriteLen int) error {
	scratch := make([]byte, shorteningBufsize)
	rewrite3 := func(a any, s3 *Shader3D) error {
		scratch = rewriteName3(s3, scratch, maxRewriteLen)
		return nil
	}
	rewrite2 := func(a any, s2 *Shader2D) error {
		scratch = rewriteName2(s2, scratch, maxRewriteLen)
		return nil
	}
	err := forEachNode(*root, rewrite3, rewrite2)
	if err != nil {
		return err
	}
	return rewrite3(nil, root)
}

func ShortenNames2D(root *Shader2D, maxRewriteLen int) error {
	scratch := make([]byte, shorteningBufsize)
	rewrite3 := func(a any, s3 *Shader3D) error {
		scratch = rewriteName3(s3, scratch, maxRewriteLen)
		return nil
	}
	rewrite2 := func(a any, s2 *Shader2D) error {
		scratch = rewriteName2(s2, scratch, maxRewriteLen)
		return nil
	}
	err := forEachNode(*root, rewrite3, rewrite2)
	if err != nil {
		return err
	}
	return rewrite2(nil, root)
}

func rewriteName3(s3 *Shader3D, scratch []byte, rewritelen int) []byte {
	sd3 := *s3
	name, scratch := makeShortname(sd3, scratch, rewritelen)
	if name == nil {
		return scratch
	}
	*s3 = &nameOverloadShader3D{Shader: sd3, name: name}
	return scratch
}

func rewriteName2(s2 *Shader2D, scratch []byte, rewritelen int) []byte {
	sd2 := *s2
	name, scratch := makeShortname(sd2, scratch, rewritelen)
	if name == nil {
		return scratch
	}
	*s2 = &nameOverloadShader2D{Shader: sd2, name: name}
	return scratch
}

// makeNewName creates.
func makeShortname(s Shader, scratch []byte, rewritelen int) (newNameOrNil []byte, newScratch []byte) {
	var h uint64 = 0xff51afd7ed558ccd
	scratch = s.AppendShaderName(scratch[:0])
	if len(scratch) < rewritelen {
		return nil, scratch // Already short name, no need to rewrite.
	}
	newName := append([]byte{}, scratch[:rewritelen]...)
	h = hash(scratch, h)
	scratch = s.AppendShaderBody(scratch[:0])
	h = hash(scratch, h)
	newName = strconv.AppendUint(newName, h, 32)
	return newName, scratch
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
// WriteShaders does not check for repeated shader names nor long tokens which may yield errors in the GL.
func WriteShaders(w io.Writer, nodes []Shader, scratch []byte) (n int, newscratch []byte, err error) {
	if scratch == nil {
		scratch = make([]byte, 1024)
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

// AppendShaderSource appends the GL code of a single shader to the dst byte buffer.  If dst's
// capacity is grown during the writing the buffer with augmented capacity is returned. If not the same input dst is returned.
// name and body byte slices pointing to the result buffer are also returned for convenience.
func AppendShaderSource(dst []byte, s Shader) (result, name, body []byte) {
	dst = append(dst, "float "...)
	nameStart := len(dst)
	dst = s.AppendShaderName(dst)
	nameEnd := len(dst)
	_, is3D := s.(Shader3D)
	if is3D {
		dst = append(dst, "(vec3 p){\n"...)
	} else {
		dst = append(dst, "(vec2 p){\n"...)
	}
	bodyStart := len(dst)
	dst = s.AppendShaderBody(dst)
	bodyEnd := len(dst)
	dst = append(dst, "\n}\n"...)
	return dst, dst[nameStart:nameEnd], dst[bodyStart:bodyEnd]
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

func forEachNode(root Shader, fn3 func(any, *Shader3D) error, fn2 func(any, *Shader2D) error) error {
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
				return fmt.Errorf("found shader %T that does not implement Shader3D nor Shader2D", obj)
			}
			var err error
			if ok3 {
				// Got Shader3D in obj.
				err = obj3.ForEachChild(userData, func(userData any, s *Shader3D) error {
					if s == nil || *s == nil {
						return nilChild
					}
					children = append(children, *s)
					return fn3(userData, s)
				})
				if obj32, ok32 := obj.(shader3D2D); ok32 {
					// The Shader3D obj contains Shader2D children, such is case for 2D->3D operations i.e: revolution and extrusion operations.
					err = obj32.ForEach2DChild(userData, func(userData any, s *Shader2D) error {
						if s == nil || *s == nil {
							return nilChild
						}
						children = append(children, *s)
						return fn2(userData, s)
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
					return fn2(userData, s)
				})
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func AppendDistanceDecl(b []byte, floatVarname, sdfPositionArgInput string, s Shader) []byte {
	b = append(b, "float "...)
	b = append(b, floatVarname...)
	b = append(b, '=')
	b = s.AppendShaderName(b)
	b = append(b, '(')
	b = append(b, sdfPositionArgInput...)
	b = append(b, ");\n"...)
	return b
}

func AppendVec3Decl(b []byte, vec3Varname string, v ms3.Vec) []byte {
	b = append(b, "vec3 "...)
	b = append(b, vec3Varname...)
	b = append(b, "=vec3("...)
	arr := v.Array()
	b = AppendFloats(b, ',', '-', '.', arr[:]...)
	b = append(b, ')', ';', '\n')
	return b
}

func AppendVec2Decl(b []byte, vec2Varname string, v ms2.Vec) []byte {
	b = append(b, "vec2 "...)
	b = append(b, vec2Varname...)
	b = append(b, "=vec2("...)
	arr := v.Array()
	b = AppendFloats(b, ',', '-', '.', arr[:]...)
	b = append(b, ')', ';', '\n')
	return b
}

func AppendFloatDecl(b []byte, floatVarname string, v float32) []byte {
	b = append(b, "float "...)
	b = append(b, floatVarname...)
	b = append(b, '=')
	b = AppendFloat(b, '-', '.', v)
	b = append(b, ';', '\n')
	return b
}

func AppendMat2Decl(b []byte, mat2Varname string, m22 ms2.Mat2) []byte {
	arr := m22.Array()
	return appendMatDecl(b, "mat2", mat2Varname, 2, 2, arr[:])
}

func AppendMat3Decl(b []byte, mat3Varname string, m33 ms3.Mat3) []byte {
	arr := m33.Array()
	return appendMatDecl(b, "mat3", mat3Varname, 3, 3, arr[:])
}

func AppendMat4Decl(b []byte, mat4Varname string, m44 ms3.Mat4) []byte {
	arr := m44.Array()
	return appendMatDecl(b, "mat4", mat4Varname, 4, 4, arr[:])
}

func appendMatDecl(b []byte, typename, name string, row, col int, arr []float32) []byte {
	b = append(b, typename...)
	b = append(b, ' ')
	b = append(b, name...)
	b = append(b, '=')
	b = append(b, typename...)
	b = append(b, '(')
	for i := 0; i < row; i++ {
		for j := 0; j < col; j++ {
			v := arr[j*row+i] // Column major access, as per OpenGL standard.
			b = AppendFloat(b, '-', '.', v)
			last := i == row-1 && j == col-1
			if !last {
				b = append(b, ',')
			}
		}
	}
	b = append(b, ");\n"...)
	return b
}

func AppendFloat(b []byte, neg, decimal byte, v float32) []byte {
	start := len(b)
	b = strconv.AppendFloat(b, float64(v), 'f', decimalDigits, 32)
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
	// TODO(soypat): Round off when find N consecutive 9's?
	return b[:end]
}

func AppendFloats(b []byte, sep, neg, decimal byte, s ...float32) []byte {
	for i, v := range s {
		b = AppendFloat(b, neg, decimal, v)
		if sep != 0 && i != len(s)-1 {
			b = append(b, sep)
		}
	}
	return b
}

const maxLineLim = 500

func AppendFloatSliceDecl(b []byte, floatSliceVarname string, vecs []float32) []byte {
	lineStart := len(b)
	b = appendStartSliceDecl(b, "float", floatSliceVarname, len(vecs))
	for i, v := range vecs {
		last := i == len(vecs)-1
		b = AppendFloat(b, '-', '.', v)
		if !last {
			b = append(b, ',')
			lineLen := len(b) - lineStart
			if lineLen > maxLineLim {
				b = append(b, '\n') // Break up line for VERY long polygon vertex lists.
				lineStart = len(b)
			}
		}
	}
	b = append(b, ");\n"...)
	return b
}

func AppendVec2SliceDecl(b []byte, vec2Varname string, vecs []ms2.Vec) []byte {
	lineStart := len(b)
	b = appendStartSliceDecl(b, "vec2", vec2Varname, len(vecs))
	for i, v := range vecs {
		last := i == len(vecs)-1
		b = append(b, "vec2("...)
		b = AppendFloats(b, ',', '-', '.', v.X, v.Y)
		b = append(b, ')')
		if !last {
			b = append(b, ',')
			lineLen := len(b) - lineStart
			if lineLen > maxLineLim {
				b = append(b, '\n') // Break up line for VERY long polygon vertex lists.
				lineStart = len(b)
			}
		}
	}
	b = append(b, ");\n"...)
	return b
}

func AppendVec3SliceDecl(b []byte, vec3Varname string, vecs []ms3.Vec) []byte {
	lineStart := len(b)
	b = appendStartSliceDecl(b, "vec3", vec3Varname, len(vecs))
	for i, v := range vecs {
		last := i == len(vecs)-1
		b = append(b, "vec3("...)
		b = AppendFloats(b, ',', '-', '.', v.X, v.Y, v.Z)
		b = append(b, ')')
		if !last {
			b = append(b, ',')
			lineLen := len(b) - lineStart
			if lineLen > maxLineLim {
				b = append(b, '\n') // Break up line for VERY long polygon vertex lists.
				lineStart = len(b)
			}
		}
	}
	b = append(b, ");\n"...)
	return b
}

func appendStartSliceDecl(b []byte, typeName, varName string, length int) []byte {
	l := int64(length)
	typeStart := len(b)
	b = append(b, typeName...)
	b = append(b, "["...)
	b = strconv.AppendInt(b, l, 10)
	b = append(b, ']')
	typeEnd := len(b)
	b = append(b, varName...)
	b = append(b, '=')
	b = append(b, b[typeStart:typeEnd]...) // Reuse typename appended earlier.
	b = append(b, '(')
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

func (xyz XYZBits) AppendMapped_XYZ(b []byte) []byte {
	return xyz.AppendMapped(b, [3]byte{'X', 'Y', 'Z'})
}

func (xyz XYZBits) AppendMapped_xyz(b []byte) []byte {
	return xyz.AppendMapped(b, [3]byte{'x', 'y', 'z'})
}

func (xyz XYZBits) AppendMapped_rgb(b []byte) []byte {
	return xyz.AppendMapped(b, [3]byte{'r', 'g', 'b'})
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

// Evaluate implements the gleval.SDF3 interface.
func (c3 *CachedShader3D) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, ok := c3.Shader.(sdf3)
	if !ok {
		return fmt.Errorf("%T does not implement gleval.SDF3", c3.Shader)
	}
	return sdf.Evaluate(pos, dist, userData)
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

// Evaluate implements the gleval.SDF2 interface.
func (c2 *CachedShader2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, ok := c2.Shader.(sdf2)
	if !ok {
		return fmt.Errorf("%T does not implement gleval.SDF2", c2.Shader)
	}
	return sdf.Evaluate(pos, dist, userData)
}

type nameOverloadShader3D struct {
	Shader Shader3D
	name   []byte
}

// Bounds returns the cached 3D bounds. Implements [Shader3D]. Update by calling RefreshCache.
func (nos3 *nameOverloadShader3D) Bounds() ms3.Box { return nos3.Shader.Bounds() }

// ForEachChild calls the underlying Shader's ForEachChild. Implements [Shader3D].
func (nos3 *nameOverloadShader3D) ForEachChild(userData any, fn func(userData any, s *Shader3D) error) error {
	return nos3.Shader.ForEachChild(userData, fn)
}

// AppendShaderBody returns the cached Shader function body. Implements [Shader]. Update by calling RefreshCache.
func (nos3 *nameOverloadShader3D) AppendShaderBody(b []byte) []byte {
	return nos3.Shader.AppendShaderBody(b)
}

// ForEach2DChild calls the underlying Shader's ForEach2DChild. This method is called for 3D shapes that
// use 2D shaders such as extrude and revolution. Implements [Shader2D].
func (nos3 *nameOverloadShader3D) ForEach2DChild(userData any, fn func(userData any, s *Shader2D) error) (err error) {
	s2, ok := nos3.Shader.(shader3D2D)
	if ok {
		err = s2.ForEach2DChild(userData, fn)
	}
	return err
}

type (
	sdf3 interface {
		Evaluate(pos []ms3.Vec, dist []float32, userData any) error
	}
	sdf2 interface {
		Evaluate(pos []ms2.Vec, dist []float32, userData any) error
	}
)

func (nos3 *nameOverloadShader3D) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, ok := nos3.Shader.(sdf3)
	if !ok {
		return fmt.Errorf("%T does not implement gleval.SDF3", nos3.Shader)
	}
	return sdf.Evaluate(pos, dist, userData)
}

func (nos3 *nameOverloadShader3D) AppendShaderName(b []byte) []byte {
	return append(b, nos3.name...)
}

type nameOverloadShader2D struct {
	Shader Shader2D
	name   []byte
}

func (nos2 *nameOverloadShader2D) Bounds() ms2.Box { return nos2.Shader.Bounds() }

func (nos2 *nameOverloadShader2D) ForEach2DChild(userData any, fn func(userData any, s *Shader2D) error) error {
	return nos2.Shader.ForEach2DChild(userData, fn)
}

func (nos2 *nameOverloadShader2D) AppendShaderName(b []byte) []byte {
	return append(b, nos2.name...)
}

func (nos2 *nameOverloadShader2D) AppendShaderBody(b []byte) []byte {
	return nos2.Shader.AppendShaderBody(b)
}

func (nos2 *nameOverloadShader2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, ok := nos2.Shader.(sdf2)
	if !ok {
		return fmt.Errorf("%T does not implement gleval.SDF2", nos2.Shader)
	}
	return sdf.Evaluate(pos, dist, userData)
}

func hash(b []byte, in uint64) uint64 {
	// Leaving md5 here since we may need to revert to
	// a more entropic hash to avoid collisions...
	// though I don't think it'll be necessary.
	// var result [16]byte
	// h := md5.New()
	// h.Write(b)
	// h.Sum(result[:0])
	// x1 := binary.LittleEndian.Uint64(result[:])
	// x2 := binary.LittleEndian.Uint64(result[8:])
	// return x1 ^ x2 ^ in
	x := in
	for len(b) >= 8 {
		x ^= binary.LittleEndian.Uint64(b)
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		x ^= x >> 31
		b = b[8:]
	}
	for i := range b {
		x ^= uint64(b[i]) << i * 8
	}
	return x
}
