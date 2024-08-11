package glrender

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms3"
)

// WriteBinarySTL writes model triangles to a writer in STL file format.
func WriteBinarySTL(w io.Writer, model []ms3.Triangle) (int, error) {
	if len(model) == 0 {
		return 0, errors.New("empty triangle slice")
	}

	nt := int64(len(model)) // int64 cast so that next line works correctly on 32bit machines.
	if nt > math.MaxUint32 {
		return 0, errors.New("amount of triangles in model exceeds STL design limits")
	}
	header := stlHeader{
		Count: uint32(nt),
	}

	var buf [84]byte
	header.put(buf[:])
	n, err := w.Write(buf[:84])
	if err != nil {
		return n, err
	} else if n != len(buf) {
		return n, io.ErrShortWrite
	}
	var d stlTriangle
	const triangleSize = 50
	for _, triangle := range model {
		norm := ms3.Unit(triangle.Normal())
		d.Normal[0] = norm.X
		d.Normal[1] = norm.Y
		d.Normal[2] = norm.Z
		d.Vertex1[0] = triangle[0].X
		d.Vertex1[1] = triangle[0].Y
		d.Vertex1[2] = triangle[0].Z
		d.Vertex2[0] = triangle[1].X
		d.Vertex2[1] = triangle[1].Y
		d.Vertex2[2] = triangle[1].Z
		d.Vertex3[0] = triangle[2].X
		d.Vertex3[1] = triangle[2].Y
		d.Vertex3[2] = triangle[2].Z
		d.put(buf[:])
		ngot, err := w.Write(buf[:triangleSize])
		n += ngot
		if err != nil {
			return n, err
		} else if ngot != triangleSize {
			return n, io.ErrShortWrite
		}
	}
	return n, nil
}

// stlHeader defines the STL file header.
type stlHeader struct {
	_     [80]uint8 // Header
	Count uint32    // Number of triangles
}

func (h stlHeader) put(b []byte) {
	_ = b[83] //early bounds check
	binary.LittleEndian.PutUint32(b[80:], h.Count)
}

// stlTriangle defines the triangle data within an STL file.
type stlTriangle struct {
	Normal  [3]float32
	Vertex1 [3]float32
	Vertex2 [3]float32
	Vertex3 [3]float32
	_       uint16 // Attribute byte count
}

func (t stlTriangle) put(b []byte) {
	if len(b) < 50 {
		panic("need length 50 to marshal stlTriangle")
	}

	put3F32(b, t.Normal)
	put3F32(b[12:], t.Vertex1)
	put3F32(b[24:], t.Vertex2)
	put3F32(b[36:], t.Vertex3)
	binary.LittleEndian.PutUint16(b[48:], 0) // Zero out attributes.
}

func (t *stlTriangle) get(b []byte) {
	if len(b) < 50 {
		panic("need length 50 to unmarshal stlTriangle")
	}
	get3F32(b, &t.Normal)
	get3F32(b[12:], &t.Vertex1)
	get3F32(b[24:], &t.Vertex2)
	get3F32(b[36:], &t.Vertex3)
	// no attributes supported yet.
}

func put3F32(b []byte, f [3]float32) {
	_ = b[11] // early bounds check
	binary.LittleEndian.PutUint32(b, math.Float32bits(f[0]))
	binary.LittleEndian.PutUint32(b[4:], math.Float32bits(f[1]))
	binary.LittleEndian.PutUint32(b[8:], math.Float32bits(f[2]))
}

func get3F32(b []byte, f *[3]float32) {
	_ = b[11] // early bounds check
	f[0] = math.Float32frombits(binary.LittleEndian.Uint32(b))
	f[1] = math.Float32frombits(binary.LittleEndian.Uint32(b[4:]))
	f[2] = math.Float32frombits(binary.LittleEndian.Uint32(b[8:]))
}

func bad3F32(f [3]float32) bool {
	return math32.IsNaN(f[0]) || math32.IsInf(f[0], 0) ||
		math32.IsNaN(f[1]) || math32.IsInf(f[1], 0) ||
		math32.IsNaN(f[2]) || math32.IsInf(f[2], 0)
}

var errCalculatedNormalMismatch = errors.New("mismatch normal")

func (t stlTriangle) validate() error {
	const epsilon = 1e-12
	const normTol = 5e-2
	if bad3F32(t.Normal) {
		return errors.New("inf/NaN STL triangle normal")
	}
	if bad3F32(t.Vertex1) || bad3F32(t.Vertex2) || bad3F32(t.Vertex3) {
		return errors.New("inf/NaN STL triangle vertex")
	}
	if t.degenerate(epsilon) {
		return errors.New("triangle is degenerate")
	}
	gotNormal := vecFromArray(t.Normal)
	calcNormal := t.normalFromVertices()
	calcNormalNeg := ms3.Scale(-1, calcNormal)

	if !ms3.EqualElem(calcNormal, gotNormal, normTol) && !ms3.EqualElem(calcNormalNeg, gotNormal, normTol) {
		return errCalculatedNormalMismatch // sometimes may fail
	}
	return nil
}

func vecFromArray(f [3]float32) ms3.Vec {
	return ms3.Vec{X: f[0], Y: f[1], Z: f[2]}
}

func (t stlTriangle) normalFromVertices() ms3.Vec {
	v1 := ms3.Scale(10, vecFromArray(t.Vertex1))
	v2 := ms3.Scale(10, vecFromArray(t.Vertex2))
	v3 := ms3.Scale(10, vecFromArray(t.Vertex3))
	e1 := ms3.Sub(v2, v1)
	e2 := ms3.Sub(v3, v1)
	return ms3.Unit(ms3.Cross(e1, e2))
}

func (t stlTriangle) Triangle() ms3.Triangle {
	return ms3.Triangle{vecFromArray(t.Vertex1), vecFromArray(t.Vertex2), vecFromArray(t.Vertex3)}
}

// Degenerate returns true if the triangle is degenerate.
func (t stlTriangle) degenerate(tol float32) bool {
	// check for identical vertices.
	// TODO more tests needed.
	return t.Triangle().IsDegenerate(tol)
}

func ReadBinarySTL(r io.Reader) (output []ms3.Triangle, readErr error) {
	var header stlHeader
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, errors.New("encountered EOF while reading STL header")
		}
		return nil, errors.New("STL header read failed: " + err.Error())
	}
	if header.Count == 0 {
		return nil, errors.New("STL header indicates 0 triangles present")
	}
	var (
		buf            [50]byte
		d              stlTriangle
		i              int
		normMismatches int
	)
	errCalculatedNormalMismatch := errors.New("mismatch normal")
	defer func() {
		if readErr != nil && !errors.Is(readErr, errCalculatedNormalMismatch) {
			readErr = fmt.Errorf("%d/%d STL triangles read: %w", i+1, header.Count, readErr)
		}
	}()
	for i = 0; i < int(header.Count); i++ {
		var n int
		for n < 50 {
			nr, err := r.Read(buf[n:])
			if err != nil {
				return nil, err
			}
			n += nr
		}
		d.get(buf[:])
		if err := d.validate(); err != nil {
			if errors.Is(err, errCalculatedNormalMismatch) {
				normMismatches++
				if normMismatches > 10_000 {
					// This may be valid output, so we return the triangles.
					return output, fmt.Errorf("got too many normal vector mismatches (%d)", normMismatches)
				}
				readErr = err
			} else {
				return nil, err
			}
		}
		output = append(output, d.Triangle())
	}
	// NormalMismatch error validation may be returned.
	// For high resolution models this error may be incorrectly returned.
	return output, readErr
}
