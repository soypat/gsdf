package gsdf

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"unsafe"

	"github.com/chewxy/math32"
	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/glbuild"
)

const (
	// For an equilateral triangle of side length L the length of bisector is L multiplied this number which is sqrt(1-0.25).
	tribisect = 0.8660254037844386467637231707529361834714026269051903140279034897
	sqrt2d2   = math32.Sqrt2 / 2
	sqrt3     = 1.7320508075688772935274463415058723669428052538103806280558069794
	largenum  = 1e20
	// epstol is used to check for badly conditioned denominators
	// such as lengths used for normalization or transformation matrix determinants.
	epstol = 6e-7
)

// Flags is a bitmask of values to control the functioning of the [Builder] type.
type Flags uint64

const (
	// FlagNoDimensionPanic controls panicking behavior on invalid shape dimension errors.
	// If set then these errors do not panic, instead storing the error for later inspection with [Builder.Err].
	FlagNoDimensionPanic Flags = 1 << iota
	// FlagUseShaderBuffers enforces the use of shader object for all newly built
	// SDFs which require a dynamic array(s) to be rendered correctly.
	FlagUseShaderBuffers
	// FlagNoShaderBuffers if set will cause the Builder to avoid all shader buffer usage, instead depending on
	// the shader program entirely for shape definition.
	FlagNoShaderBuffers
)

// Builder wraps all SDF primitive and operation logic generation.
// Provides error handling strategies with panics or error accumulation during shape generation.
type Builder struct {
	// flags is a bitfield controlling behaviour of Builder.
	flags     Flags
	accumErrs []error
	// limVecGPU
	limVecGPU int
}

// useShaderBuffer enables selection of GPU over CPU algorithm depending on Builder configuration.
func (bld *Builder) useShaderBuffer(components int) bool {
	avoidGPU := bld.flags&FlagNoShaderBuffers != 0
	if avoidGPU {
		return false
	}
	lim := bld.limVecGPU
	if lim == 0 {
		lim = 128 // Beware: Older GPUs typically support a maximum of 1024 components (float32s).
	}
	useGPU := bld.flags&FlagUseShaderBuffers != 0
	return useGPU || components > lim
}

func makeHashName[T any](dst []byte, name string, vec []T) []byte {
	var z T
	data := unsafe.Pointer(&vec[0])
	sz := len(vec) * int(unsafe.Sizeof(z))
	return fmt.Appendf(dst, "%s%x_%x", name, uintptr(data), sz)
}

func (bld *Builder) Flags() Flags {
	return bld.flags
}

func (bld *Builder) SetFlags(flags Flags) error {
	avoidGPU := bld.flags&FlagNoShaderBuffers != 0
	useGPU := bld.flags&FlagUseShaderBuffers != 0
	if avoidGPU && useGPU {
		return errors.New("invalid flag setup: both use/avoid shader buffer bits set")
	}
	bld.flags = flags
	return nil
}

// Err returns errors accumulated during SDF primitive creation and operations. The returned error implements `Unwrap() []error`.
func (bld *Builder) Err() error {
	if len(bld.accumErrs) == 0 {
		return nil
	}
	return errors.Join(bld.accumErrs...)
}

// ClearErrors clears accumulated errors such that [Builder.Err] returns nil on next call.
func (bld *Builder) ClearErrors() {
	bld.accumErrs = bld.accumErrs[:0]
}

func (bld *Builder) shapeErrorf(msg string, args ...any) {
	if bld.flags&FlagNoDimensionPanic == 0 {
		panic(fmt.Sprintf(msg, args...))
	}
	// bld.stacks = append(bld.stacks, string(debug.Stack()))
	bld.accumErrs = append(bld.accumErrs, fmt.Errorf(msg, args...))
}

func (*Builder) nilsdf(msg string) {
	panic("nil SDF argument: " + msg)
}

func onBuildOp[T glbuild.Shader](bld *Builder, s T) T {
	return s
}

func onBuildPrimitive[T glbuild.Shader](bld *Builder, s T) T {
	return s
}

func extractReturnExpression(returnExpr []byte) ([]byte, error) {
	returnExpr = bytes.TrimSpace(returnExpr)
	returnExpr = bytes.TrimPrefix(returnExpr, []byte("return "))
	if len(returnExpr) < 3 {
		return nil, errors.New("too short return expression")
	}
	returnExpr = bytes.TrimSpace(returnExpr)
	idx := bytes.IndexByte(returnExpr, ';')
	if idx != len(returnExpr)-1 {
		return nil, errors.New("return expression must have one single semicolon at end")
	}
	return returnExpr, nil
}

// These interfaces are implemented by all SDF interfaces such as SDF3/2 and Shader3D/2D.
// Using these instead of `any` Aids in catching mistakes at compile time such as passing a Shader3D instead of Shader2D as an argument.
type (
	bounder2 = interface{ Bounds() ms2.Box }
	bounder3 = interface{ Bounds() ms3.Box }
)

func minf(a, b float32) float32 {
	return math32.Min(a, b)
}
func hypotf(a, b float32) float32 {
	return math32.Hypot(a, b)
}

func signf(a float32) float32 {
	if a == 0 {
		return 0
	}
	return math32.Copysign(1, a)
}

func clampf(v, Min, Max float32) float32 {
	// return ms3.Clamp(v, Min, Max)
	if v < Min {
		return Min
	} else if v > Max {
		return Max
	}
	return v
}

func mixf(x, y, a float32) float32 {
	return x*(1-a) + y*a
}

func maxf(a, b float32) float32 {
	return math32.Max(a, b)
}

func absf(a float32) float32 {
	return math32.Abs(a)
}

// ndot returns the negative dot product ax*bx - ay*by
func ndot(a, b ms2.Vec) float32 {
	return a.X*b.X - a.Y*b.Y
}

func powelem2(k float32, a ms2.Vec) ms2.Vec {
	return ms2.Vec{X: math32.Pow(a.X, k), Y: math32.Pow(a.Y, k)}
}

func cos_acos_3(x float32) float32 {
	x = math32.Sqrt(0.5 + 0.5*x)
	return x*(x*(x*(x*-0.008972+0.039071)-0.107074)+0.576975) + 0.5
}

func hash2vec2(vecs ...[2]ms2.Vec) float32 {
	var hashA float32 = 0.0
	var hashB float32 = 1.0
	for _, v := range vecs {
		hashA, hashB = hashAdd(hashA, hashB, v[0].X)
		hashA, hashB = hashAdd(hashA, hashB, v[0].Y)
		hashA, hashB = hashAdd(hashA, hashB, v[1].X)
		hashA, hashB = hashAdd(hashA, hashB, v[1].Y)
	}
	return hashfint(hashA + hashB)
}

func hashf(values []float32) float32 {
	var hashA float32 = 0.0
	var hashB float32 = 1.0
	for _, num := range values {
		hashA, hashB = hashAdd(hashA, hashB, num)
	}
	return hashfint(hashA + hashB)
}

func hashAdd(a, b, num float32) (aNew, bNew float32) {
	const prime = 31.0
	a += num
	b *= (prime + num)
	a = hashfint(a)
	b = hashfint(b)
	return a, b
}

func hashfint(f float32) float32 {
	return float32(int(f*1000000)%1000000) / 1000000 // Keep within [0.0, 1.0)
}

// func hash(b []byte, in uint64) uint64 {
// 	x := in
// 	for len(b) >= 8 {
// 		x ^= binary.LittleEndian.Uint64(b)
// 		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
// 		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
// 		x ^= x >> 31
// 		b = b[8:]

// 	}
// 	if len(b) > 0 {
// 		var buf [8]byte
// 		copy(buf[:], b)
// 		x ^= binary.LittleEndian.Uint64(buf[:])
// 		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
// 		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
// 		x ^= x >> 31
// 	}
// 	return x
// }

func hashshaderptr(s glbuild.Shader) uint64 {
	v := *(*[2]uintptr)(unsafe.Pointer(&s))
	return (uint64(v[0]) ^ (uint64(v[1]) << 8)) * 0xbf58476d1ce4e5b9
}

// appendTypicalReturnFuncCall appends a function call of the following style to the dst buffer and returns the result:
//
//	return <funcname>([firstArgs], [s0], [s1]...);
func appendTypicalReturnFuncCall(dst []byte, funcname string, firstArgs string, s ...float32) []byte {
	dst = append(dst, "return "...)
	dst = append(dst, funcname...)
	dst = append(dst, '(')
	dst = append(dst, firstArgs...)
	if len(s) > 0 {
		if len(firstArgs) != 0 {
			dst = append(dst, ',')
		}
		dst = glbuild.AppendFloats(dst, ',', '-', '.', s...)
	}
	dst = append(dst, ");"...)
	return dst
}
