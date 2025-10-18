package glbuild

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/soypat/geometry/md2"
	"github.com/soypat/geometry/md3"
	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
)

const VersionStr = "#version 430\n"

// Shader stores information for automatically generating SDF Shader pipelines
// and evaluating them correctly on a GPU.
type Shader interface {
	// AppendShaderName appends the name of the GL shader function
	// to the buffer and returns the result. It should be unique to that shader.
	AppendShaderName(b []byte) []byte
	// AppendShaderBody appends the body of the shader function to the
	// buffer and returns the result.
	AppendShaderBody(b []byte) []byte
	// AppendShaderObject appends "objects" (read as data) needed to
	// evaluate the shader correctly. See [ShaderObject] for more information
	// on what an object can represent.
	AppendShaderObjects(objs []ShaderObject) []ShaderObject
}

// ShaderObject is a handle to data needed to evaluate a [Shader] correctly.
// A ShaderObject could represent any of the following:
//   - Shader Storage Buffer Object (SSBO). Is a 1D array of structured data.
//   - Texture. Represents 2D data, usually images.
//   - Shader uniform. Is a single structured value.
type ShaderObject struct {
	// NamePtr is a pointer to the name of the buffer inside of the [Shader].
	// This lets the programmer edit the name if a naming conflict is found before generating the shader bodies.
	NamePtr []byte

	// Element is the element type of the buffer.
	Element reflect.Type
	// Data points to the start of buffer data.
	Data unsafe.Pointer
	// Size of buffer in bytes.
	Size int
	// Binding specifies the resource's binding point during shader execution.
	// Binding should be equal to -1 until the final binding point is allocated in shader generation.
	Binding int
	read    bool
	// for function shaders.
	funcSource []byte
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
	objsScratch   []ShaderObject
	// names maps shader names to body hashes for checking duplicates.
	names map[uint64]uint64
	// Invocations size in X (local group size) to give each compute work group.
	invocX int
}

func MakeShaderFunction(shaderDef []byte) (sf ShaderObject, err error) {
	shaderDef = bytes.TrimSpace(shaderDef)
	fnNameEnd := bytes.IndexByte(shaderDef, '(')
	fnNameStart := bytes.IndexByte(shaderDef, ' ')
	if fnNameEnd < 0 || fnNameStart < 0 || fnNameStart > fnNameEnd {
		return ShaderObject{}, errors.New("unable to parse function name")
	}
	name := shaderDef[fnNameStart:fnNameEnd]
	name = bytes.TrimSpace(name)
	if len(name) == 0 {
		return ShaderObject{}, errors.New("empty function name")
	}
	sf = ShaderObject{
		NamePtr:    name,
		funcSource: shaderDef,
		Binding:    -1,
	}
	return sf, nil
}

func (ssbo ShaderObject) IsFunction() bool { return len(ssbo.funcSource) > 0 }
func (ssbo ShaderObject) IsBindable() bool { return !ssbo.IsFunction() }

func MakeShaderBufferReadOnly[T any](namePtr []byte, data []T) (ssbo ShaderObject, err error) {
	var z T
	ssbo = ShaderObject{
		NamePtr: namePtr,
		Element: reflect.TypeOf(z),
		Data:    unsafe.Pointer(&data[0]),
		Size:    int(unsafe.Sizeof(z)) * len(data),
		read:    true,
	}
	err = ssbo.Validate()
	if err != nil {
		return ShaderObject{}, err
	}
	// Until shader pipeline we do not know where our buffer will be binded.
	// Programmer expects -1 binding until then.
	ssbo.Binding = -1
	return ssbo, nil
}

var defaultComputeHeader = []byte("#shader compute\n" + VersionStr)

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
func (p *Programmer) WriteComputeSDF3(w io.Writer, obj Shader3D) (int, []ShaderObject, error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], obj)
	if err != nil {
		return 0, nil, err
	}
	// Begin writing shader source code.
	n, err := w.Write(p.computeHeader)
	if err != nil {
		return n, nil, err
	}
	ngot, objs, err := p.writeShaders(w, nodes)
	n += ngot
	if err != nil {
		return n, nil, err
	}
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
	return n, objs, err
}

// WriteDistanceIO creates the bare bones I/O compute program for calculating SDF
// and writes it to the writer.
func (p *Programmer) WriteComputeSDF2(w io.Writer, obj Shader2D) (int, []ShaderObject, error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], obj)
	if err != nil {
		return 0, nil, err
	}
	// Begin writing shader source code.
	n, err := w.Write(p.computeHeader)
	if err != nil {
		return n, nil, err
	}
	ngot, objs, err := p.writeShaders(w, nodes)
	n += ngot
	if err != nil {
		return n, objs, err
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
	return n, objs, err
}

//go:embed visualizer_footer.tmpl
var shaderToyVisualFooter []byte

// WriteShaderToyVisualizerSDF3 generates a OpenGL program that can be visualized in most shader visualizers such as ShaderToy.
func (p *Programmer) WriteShaderToyVisualizerSDF3(w io.Writer, obj Shader3D) (n int, objs []ShaderObject, err error) {
	baseName, n, objs, err := p.WriteSDFDecl(w, obj)
	if err != nil {
		return 0, objs, err
	}
	for i := range objs {
		if objs[i].IsBindable() {
			return n, objs, errors.New("visualization shader does not support binding SSBOs. Create your SDFs with no shader buffers by unsetting FlagUseShaderBuffers in gsdf.Builder flags")
		}
	}
	ngot, err := w.Write([]byte("\nfloat sdf(vec3 p) { return " + baseName + "(p); }\n\n"))
	n += ngot
	if err != nil {
		return n, objs, err
	}
	ngot, err = w.Write(shaderToyVisualFooter)
	n += ngot
	if err != nil {
		return n, objs, err
	}
	return n, objs, nil
}

// WriteShaderDecl writes the SDF shader function declarations and returns the top-level SDF function name.
func (p *Programmer) WriteSDFDecl(w io.Writer, s Shader) (baseName string, n int, objs []ShaderObject, err error) {
	baseName, nodes, err := ParseAppendNodes(p.scratchNodes[:0], s)
	if err != nil {
		return "", 0, nil, err
	}
	n, objs, err = p.writeShaders(w, nodes)
	if err != nil {
		return "", n, objs, err
	}
	return baseName, n, objs, nil
}

func (p *Programmer) writeShaders(w io.Writer, nodes []Shader) (n int, objs []ShaderObject, err error) {
	clear(p.names)
	p.scratch = p.scratch[:0]
	p.objsScratch = p.objsScratch[:0]
	const startBase = 2
	currentBase := startBase
	objIdx := 0
	for i := len(nodes) - 1; i >= 0; i-- {
		// Start by generating all Shader Objects.
		node := nodes[i]
		p.objsScratch = node.AppendShaderObjects(p.objsScratch)
		newObjs := p.objsScratch[objIdx:]
	OBJWRITE:
		for i := range newObjs {
			obj := &newObjs[i]
			if obj.Binding != -1 {
				return n, nil, fmt.Errorf("shader buffer object binding should be set to -1 until shader generated for %T, %q", unwraproot(node), obj.NamePtr)
			}
			nameHash := hash(obj.NamePtr, 0)
			_, nameConflict := p.names[nameHash]
			if nameConflict {
				oldObjs := p.objsScratch[:objIdx]
				for _, old := range oldObjs {
					conflictFound := nameHash == hash(old.NamePtr, 0)
					if !conflictFound {
						continue
					}
					if obj.IsFunction() && bytes.Equal(obj.funcSource, old.funcSource) {
						continue OBJWRITE // Skip this function, is duplicate.
					} else if obj.IsFunction() {
						type ShaderFunction uint8
						obj.Element = reflect.TypeOf(ShaderFunction(0))
						break // conflicting function name.
					}
					// Conflict found!
					if obj.Data == old.Data && obj.Size == old.Size && obj.Element == old.Element {
						continue OBJWRITE // Skip this object, is duplicate and already has been added.
					}
					break // Conflict is not identical.
				}
				return n, nil, fmt.Errorf("shader buffer object name conflict resolution not implemented: %T has buffer conflicting name %q of type %s", unwraproot(node), obj.NamePtr, obj.Element.String())
			}
			obj.Binding = currentBase
			currentBase++
			p.names[nameHash] = nameHash
			blockName := string(obj.NamePtr) + "Buffer"
			p.scratch, err = AppendShaderBufferDecl(p.scratch, blockName, "", *obj)
			if err != nil {
				return n, nil, err
			}
		}
		objIdx += len(newObjs)
	}

	if len(p.scratch) > 0 {
		// Write shader buffer declarations if any.
		ngot, err := w.Write(p.scratch)
		n += ngot
		if err != nil {
			return n, nil, err
		}
	}

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
			// Look for identical shader
			var conflictBody []byte
			for j := i + 1; j < len(nodes); j++ {
				conflictBody = nodes[j].AppendShaderName(conflictBody[:0])
				if bytes.Equal(conflictBody, name) {
					conflictBody = nodes[j].AppendShaderBody(conflictBody[:0])
					break
				}
				conflictBody = conflictBody[:0]
			}
			return n, nil, fmt.Errorf("duplicate %T shader name %q w/ body:\n%s\n\nconflict with distinct shader with same name:\n%s", unwraproot(node), name, body, conflictBody)
		} else {
			p.names[nameHash] = bodyHash // Not found, add it.
		}
		ngot, err := w.Write(p.scratch)
		n += ngot
		if err != nil {
			return n, nil, err
		}
	}
	objs = append(objs[:0], p.objsScratch...) // Clone slice and return it.
	return n, objs, err
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
	err := forEachNodeBFS(*root, rewrite3, rewrite2)
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
	err := forEachNodeBFS(*root, rewrite3, rewrite2)
	if err != nil {
		return err
	}
	return rewrite2(nil, root)
}

func rewriteName3(s3 *Shader3D, scratch []byte, rewritelen int) []byte {
	sd3 := *s3
	if _, ok := sd3.(*nameOverloadShader3D); ok {
		return scratch // Already overloaded.
	}
	name, scratch := makeShortname(sd3, scratch, rewritelen)
	if name == nil {
		return scratch
	}
	*s3 = &nameOverloadShader3D{Shader: sd3, name: name}
	return scratch
}

func rewriteName2(s2 *Shader2D, scratch []byte, rewritelen int) []byte {
	sd2 := *s2
	if _, ok := sd2.(*nameOverloadShader2D); ok {
		return scratch // Already overloaded.
	}
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

// AppendShaderBufferDecl appends the [ShaderObject] as a Shader Storage Buffer Object (SSBO). Returns an error if not a buffer.
//
//	layout(<ssbo.std>, binding = <base>) buffer <BlockName> {
//		<ssbo.Element> <ssbo.NamePtr>[];
//	} <instanceName>;
func AppendShaderBufferDecl(dst []byte, BlockName, instanceName string, ssbo ShaderObject) ([]byte, error) {
	err := ssbo.Validate()
	if err != nil {
		return dst, err
	} else if BlockName == "" && instanceName == "" {
		return nil, errors.New("AppendShaderBufferDecl requires BlockName for a valid SSBO declaration")
	} else if ssbo.funcSource != nil {
		dst = append(dst, '\n')
		dst = append(dst, ssbo.funcSource...)
		dst = append(dst, '\n')
		return dst, nil
	}

	typename, std, err := glTypename(ssbo.Element)
	if err != nil {
		return dst, fmt.Errorf("typename failed for %q: %w", ssbo.NamePtr, err)
	}
	dst = append(dst, "layout("...)
	dst = append(dst, std...)
	dst = append(dst, ",binding="...)
	dst = strconv.AppendInt(dst, int64(ssbo.Binding), 10)
	dst = append(dst, ") buffer"...)
	if len(BlockName) > 0 {
		dst = append(dst, ' ')
		dst = append(dst, BlockName...)
	}
	dst = append(dst, " {\n\t"...)
	dst = append(dst, typename...)
	dst = append(dst, ' ')
	dst = append(dst, ssbo.NamePtr...)
	dst = append(dst, "[];\n}"...)
	if len(instanceName) > 0 {
		dst = append(dst, ' ')
		dst = append(dst, instanceName...)
	}
	dst = append(dst, ";\n"...)
	return dst, nil
}

func (obj ShaderObject) Validate() error {
	if len(obj.NamePtr) == 0 {
		return errors.New("shader object zero-length name")
	} else if len(obj.funcSource) > 0 {
		return nil // Functions only have one required field besides NamePtr
	}
	if obj.Data == nil {
		return errors.New("shader object nil data pointer")
	} else if obj.Size == 0 {
		return errors.New("shader object zero/negative length data")
	} else if obj.Size < 0 {
		return errors.New("shader object negative length of data")
	} else if !obj.read {
		return errors.New("shader object no usage defined")
	} else if obj.Binding < 0 {
		return errors.New("shader object negative binding point")
	}
	_, _, err := glTypename(obj.Element)
	if err != nil {
		return err
	}
	return nil
}

func glTypename(tp reflect.Type) (typename, std string, err error) {
	std = "std430"
	switch tp {
	case reflect.TypeOf(md2.Vec{}):
		typename = "dvec2"
	case reflect.TypeOf(md3.Vec{}):
		typename = "dvec3"
	case reflect.TypeOf(float64(0)):
		typename = "double"
	case reflect.TypeOf(float32(0)):
		typename = "float"
	case reflect.TypeOf(ms2.Vec{}):
		typename = "vec2"
	case reflect.TypeOf(ms3.Vec{}):
		typename = "vec3"
	case reflect.TypeOf([2]ms2.Vec{}), reflect.TypeOf(ms3.Quat{}):
		typename = "vec4"
	case reflect.TypeOf(ms2.Mat2{}):
		typename = "mat2"
	case reflect.TypeOf(ms3.Mat3{}):
		typename = "mat3"
	case reflect.TypeOf(ms3.Mat4{}):
		typename = "mat4"
	case reflect.TypeOf(uint32(0)):
		typename = "uint"
	case reflect.TypeOf(int32(0)):
		typename = "int"
	case reflect.TypeOf([2]uint32{}):
		typename = "uvec2"
	case reflect.TypeOf([2]int32{}):
		typename = "ivec2"
	case reflect.TypeOf([3]uint32{}):
		typename = "uvec3"
	case reflect.TypeOf([3]int32{}):
		typename = "ivec3"
	case nil:
		err = errors.New("nil element type")
	default:
		err = fmt.Errorf("equivalent type not implemented for %s", tp.String())
	}
	return typename, std, err
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
	// found := make(map[Shader]struct{})
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
					// if _, skip := found[*s]; skip {
					// 	return nil
					// }
					// found[*s] = struct{}{}
					children = append(children, *s)
					return nil
				})
				if obj32, ok32 := obj.(shader3D2D); ok32 {
					// The Shader3D obj contains Shader2D children, such is case for 2D->3D operations i.e: revolution and extrusion operations.
					err = obj32.ForEach2DChild(userData, func(userData any, s *Shader2D) error {
						if s == nil || *s == nil {
							return nilChild
						}
						// if _, skip := found[*s]; skip {
						// 	return nil
						// }
						// found[*s] = struct{}{}
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
					// if _, skip := found[*s]; skip {
					// 	return nil
					// }
					// found[*s] = struct{}{}
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

func forEachNodeBFS(root Shader, fn3 func(userData any, s3 *Shader3D) error, fn2 func(userData any, s2 *Shader2D) error) error {
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

func forEachNodeDFS(obj Shader, fnEnter3, fnExit3 func(s3 Shader3D) error, fnEnter2, fnExit2 func(s2 Shader2D) error) (err error) {
	var userData any
	obj3, ok3 := obj.(Shader3D)
	obj2, ok2 := obj.(Shader2D)
	if !ok2 && !ok3 {
		return fmt.Errorf("found shader %T that does not implement Shader3D nor Shader2D", obj)
	}
	if ok3 {
		err = fnEnter3(obj3)
		if err != nil {
			return err
		}
		err = obj3.ForEachChild(userData, func(userData any, s *Shader3D) error {
			return forEachNodeDFS(*s, fnEnter3, fnExit3, fnEnter2, fnExit2) // TODO: try non-recursive attempt to not stack overflow... But is hard to implement.
		})
	}
	if ok2 && err == nil {
		err = fnEnter2(obj2)
		if err != nil {
			return err
		}
		err = obj2.ForEach2DChild(userData, func(userData any, s *Shader2D) error {
			return forEachNodeDFS(*s, fnEnter3, fnExit3, fnEnter2, fnExit2)
		})
		if err != nil {
			return err
		}
		err = fnExit2(obj2)
	}
	if ok3 && err == nil {
		err = fnExit3(obj3)
	}
	return err
}

func countDirectChildren(obj Shader) (directChildren int) {
	obj3, ok3 := obj.(Shader3D)
	obj2, ok2 := obj.(Shader2D)
	if ok3 {
		obj3.ForEachChild(nil, func(userData any, s *Shader3D) error {
			directChildren++
			return nil
		})
	}
	if ok2 {
		obj2.ForEach2DChild(nil, func(userData any, s *Shader2D) error {
			directChildren++
			return nil
		})
	}
	return directChildren
}

func AppendDefineDecl(b []byte, aliasToDefine, aliasReplace string) []byte {
	b = append(b, "#define "...)
	b = append(b, aliasToDefine...)
	b = append(b, ' ')
	b = append(b, aliasReplace...)
	b = append(b, '\n')
	return b
}

func AppendUndefineDecl(b []byte, aliasToUndefine string) []byte {
	b = append(b, "#undef "...)
	b = append(b, aliasToUndefine...)
	b = append(b, '\n')
	return b
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

func AppendIntDecl(b []byte, intVarname string, v int) []byte {
	b = append(b, "int "...)
	b = append(b, intVarname...)
	b = append(b, '=')
	b = strconv.AppendInt(b, int64(v), 10)
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

const decimalDigits = 9

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
	return AppendGenericSliceDecl(b, "float", floatSliceVarname, len(vecs), func(b []byte, i int) []byte {
		return AppendFloat(b, '-', '.', vecs[i])
	})
}

func AppendVec2SliceDecl(b []byte, vec2Varname string, vecs []ms2.Vec) []byte {
	return AppendGenericSliceDecl(b, "vec2", vec2Varname, len(vecs), func(b []byte, i int) []byte {
		v := vecs[i]
		b = append(b, "vec2("...)
		b = AppendFloats(b, ',', '-', '.', v.X, v.Y)
		b = append(b, ')')
		return b
	})
}

func AppendVec3SliceDecl(b []byte, vec3Varname string, vecs []ms3.Vec) []byte {
	return AppendGenericSliceDecl(b, "vec3", vec3Varname, len(vecs), func(b []byte, i int) []byte {
		v := vecs[i]
		b = append(b, "vec3("...)
		b = AppendFloats(b, ',', '-', '.', v.X, v.Y, v.Z)
		b = append(b, ')')
		return b
	})
}

func AppendGenericSliceDecl(b []byte, typename, varname string, nelem int, appendElement func(b []byte, i int) []byte) []byte {
	lineStart := len(b)
	b = appendStartSliceDecl(b, typename, varname, nelem)
	for i := 0; i < nelem; i++ {
		last := i == nelem-1
		b = appendElement(b, i)
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
	b = append(b, ' ')
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

// OverloadShader3DBounds overloads a [Shader3D] Bounds method with the argument bounding box.
func OverloadShader3DBounds(s Shader3D, bb ms3.Box) Shader3D {
	return &overloadBounds3{
		Shader3D: s,
		bb:       bb,
	}
}

type overloadBounds3 struct {
	Shader3D
	bb ms3.Box
}

func (ob3 *overloadBounds3) Bounds() ms3.Box { return ob3.bb }

// Evaluate implements the gleval.SDF3 interface.
func (ob3 *overloadBounds3) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, ok := ob3.Shader3D.(sdf3)
	if !ok {
		return fmt.Errorf("%T does not implement gleval.SDF3", ob3.Shader3D)
	}
	return sdf.Evaluate(pos, dist, userData)
}
func (ob3 *overloadBounds3) unwrap() Shader { return ob3.Shader3D }

// OverloadShader2DBounds overloads a [Shader2D] Bounds method with the argument bounding box.
func OverloadShader2DBounds(s Shader2D, bb ms2.Box) Shader2D {
	return &overloadBounds2{
		Shader2D: s,
		bb:       bb,
	}
}

type overloadBounds2 struct {
	Shader2D
	bb ms2.Box
}

func (ob2 *overloadBounds2) Bounds() ms2.Box { return ob2.bb }

// Evaluate implements the gleval.SDF2 interface.
func (ob3 *overloadBounds2) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, ok := ob3.Shader2D.(sdf2)
	if !ok {
		return fmt.Errorf("%T does not implement gleval.SDF3", ob3.Shader2D)
	}
	return sdf.Evaluate(pos, dist, userData)
}

func (ob2 *overloadBounds2) unwrap() Shader { return ob2.Shader2D }

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

// AppendShaderObjects returns the underlying [Shader]'s buffer declarations.
func (c3 *CachedShader3D) AppendShaderObjects(objs []ShaderObject) []ShaderObject {
	return c3.Shader.AppendShaderObjects(objs)
}

// Evaluate implements the gleval.SDF3 interface.
func (c3 *CachedShader3D) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, ok := c3.Shader.(sdf3)
	if !ok {
		return fmt.Errorf("%T does not implement gleval.SDF3", c3.Shader)
	}
	return sdf.Evaluate(pos, dist, userData)
}

func (c3 *CachedShader3D) unwrap() Shader { return c3.Shader }

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

// AppendShaderObjects returns the underlying [Shader]'s buffer declarations.
func (c2 *CachedShader2D) AppendShaderObjects(objs []ShaderObject) []ShaderObject {
	return c2.Shader.AppendShaderObjects(objs)
}

func (c2 *CachedShader2D) unwrap() Shader { return c2.Shader }

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

// AppendShaderObjects returns the underlying [Shader]'s buffer declarations.
func (nos3 *nameOverloadShader3D) AppendShaderObjects(objs []ShaderObject) []ShaderObject {
	return nos3.Shader.AppendShaderObjects(objs)
}

// mirrors of gleval.SDF3 and gleval.SDF2 interfaces to avoid cyclic dependencies.
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

func (nos3 *nameOverloadShader3D) unwrap() Shader { return nos3.Shader }

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

// AppendShaderObjects returns the underlying [Shader]'s buffer declarations.
func (nos2 *nameOverloadShader2D) AppendShaderObjects(objs []ShaderObject) []ShaderObject {
	return nos2.Shader.AppendShaderObjects(objs)
}

func (nos2 *nameOverloadShader2D) unwrap() Shader { return nos2.Shader }

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
	if len(b) > 0 {
		var buf [8]byte
		copy(buf[:], b)
		x ^= binary.LittleEndian.Uint64(buf[:])
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		x ^= x >> 31
	}
	return x
}

func unwraproot(s Shader) Shader {
	i := 0
	var sbase Shader
	for s != nil && i < 6 {
		sbase = s
		s = unwrap(s)
		i++
	}
	return sbase
}

func unwrap(s Shader) Shader {
	if unwrapper, ok := s.(interface{ unwrap() Shader }); ok {
		return unwrapper.unwrap()
	}
	return nil
}

func FormatShader(sh Shader) string {
	if sh == nil {
		panic("nil shader")
	}
	prevWasPrimitive := false
	var sb strings.Builder
	enterShader := func(s Shader) {
		if prevWasPrimitive {
			sb.WriteByte(',')
		}
		tp := reflect.TypeOf(s)
		if tp.Kind() == reflect.Pointer {
			tp = tp.Elem()
		}
		name := tp.Name()
		sb.WriteString(name)
		isPrimitive := countDirectChildren(s) == 0
		if !isPrimitive {
			sb.WriteByte('(')
		}
	}
	exitShader := func(s Shader) {
		isPrimitive := countDirectChildren(s) == 0
		if !isPrimitive {
			sb.WriteByte(')')
		}
		prevWasPrimitive = isPrimitive
	}
	err := forEachNodeDFS(sh, func(sd Shader3D) error {
		enterShader(sd)
		return nil
	}, func(sd Shader3D) error {
		exitShader(sd)
		return nil
	}, func(sd Shader2D) error {
		enterShader(sd)
		return nil
	}, func(sd Shader2D) error {
		exitShader(sd)
		return nil
	})
	if err != nil {
		return err.Error()
	}
	return sb.String()
}
