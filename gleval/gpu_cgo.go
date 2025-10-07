//go:build !tinygo && cgo

package gleval

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/glgl/v4.1-core/glgl"
	"github.com/soypat/gsdf/glbuild"
)

func (b *Batcher) runUnion(dst, A, B []float32) error {
	return b.runBinop("return min(a,b);", b.cfg, dst, A, B)
}

func (b *Batcher) runDiff(dst, A, B []float32) error {
	return b.runBinop("return max(a,-b);", b.cfg, dst, A, B)
}
func (b *Batcher) runIntersect(dst, A, B []float32) error {
	return b.runBinop("return max(a,b);", b.cfg, dst, A, B)
}

func (b *Batcher) runBinop(binopBody string, cfg ComputeConfig, dst, A, B []float32) error {
	if len(dst) != len(A) || len(A) != len(B) {
		return errors.New("unequal buffer lengths")
	}
	b.shaderStore = fmt.Appendf(b.shaderStore[:0], baseBinOpShader, cfg.InvocX, binopBody)
	b.shaderStore = append(b.shaderStore, 0)
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: string(b.shaderStore)})
	if err != nil {
		return err
	}
	prog.Bind()
	defer prog.Delete()
	defer prog.Unbind()
	var p runtime.Pinner
	ssboA := loadSSBO(A, 0, gl.STATIC_DRAW)
	ssboB := loadSSBO(B, 1, gl.STATIC_DRAW)
	ssboOut := createSSBO(elemSize[float32]()*len(dst), 2, gl.DYNAMIC_READ)
	p.Pin(&ssboA)
	p.Pin(&ssboB)
	p.Pin(&ssboOut)
	defer p.Unpin()
	defer gl.DeleteBuffers(1, &ssboA)
	defer gl.DeleteBuffers(1, &ssboB)
	defer gl.DeleteBuffers(1, &ssboOut)
	err = glgl.Err()
	if err != nil {
		return err
	}
	nWorkX := (len(dst) + cfg.InvocX - 1) / cfg.InvocX
	gl.DispatchCompute(uint32(nWorkX), 1, 1)
	err = glgl.Err()
	if err != nil {
		return err
	}
	gl.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT)
	err = glgl.Err()
	if err != nil {
		return err
	}
	err = copySSBO(dst, ssboOut)
	if err != nil {
		return err
	}
	return nil
}

func (lines *Lines2DGPU) evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	} else if lines.prog.ID() == 0 {
		return errors.New("program id is 0, did you configure Lines2DGPU?")
	}
	prog := lines.prog
	prog.Bind()
	defer prog.Unbind()
	loc, err := prog.UniformLocation("WidthOffset\x00")
	if err != nil {
		return err
	}
	err = prog.SetUniformf(loc, lines.Width/2)
	if err != nil {
		return err
	}

	var p runtime.Pinner
	ssbo := loadSSBO(lines.Lines, 2, gl.STATIC_DRAW)
	if ssbo == 0 {
		return glErrOrMessage("loading lines SSBO got zero id")
	}
	p.Pin(&ssbo)
	defer p.Unpin()
	defer gl.DeleteBuffers(1, &ssbo)

	err = computeEvaluate(pos, dist, lines.invocX, nil)
	if err != nil {
		return err
	}
	return nil
}

func (poly *PolygonGPU) evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	} else if poly.prog.ID() == 0 {
		return errors.New("bad program compile or PolygonGPU not initialized before first use")
	}
	prog := poly.prog
	prog.Bind()
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadSSBO(poly.Vertices, 2, gl.STATIC_DRAW)
	if ssbo == 0 {
		return glErrOrMessage("loading polygon vertices SSBO")
	}
	p.Pin(&ssbo)
	defer p.Unpin()
	defer gl.DeleteBuffers(1, &ssbo)
	err = computeEvaluate(pos, dist, poly.invocX, nil)
	if err != nil {
		return err
	}
	return nil
}

func (lines *DisplaceMulti2D) evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	} else if lines.prog.ID() == 0 {
		return errors.New("bad compile or need to initialize LinesGPU before first use")
	}

	prog := lines.prog
	prog.Bind()
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadSSBO(lines.Displacements, 2, gl.STATIC_DRAW)
	if ssbo == 0 {
		return glErrOrMessage("loading displacements SSBO got zero id")
	}
	p.Pin(&ssbo)
	defer p.Unpin()
	defer gl.DeleteBuffers(1, &ssbo)

	err = computeEvaluate(pos, dist, lines.invocX, nil)
	if err != nil {
		return err
	}
	return nil
}

func loadSSBO[T any](slice []T, base, usage uint32) (ssbo uint32) {
	var p runtime.Pinner
	p.Pin(&ssbo)
	gl.GenBuffers(1, &ssbo)
	p.Unpin()
	gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, ssbo)
	size := len(slice) * elemSize[T]()
	gl.BufferData(gl.SHADER_STORAGE_BUFFER, size, unsafe.Pointer(&slice[0]), usage)
	gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, base, ssbo)
	return ssbo
}

func createSSBO(size int, base, usage uint32) (ssbo uint32) {
	gl.GenBuffers(1, &ssbo)
	gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, ssbo)
	gl.BufferData(gl.SHADER_STORAGE_BUFFER, size, nil, usage)
	gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, base, ssbo)
	return ssbo
}

func copySSBO[T any](dst []T, ssbo uint32) error {
	singleSize := elemSize[T]()
	bufSize := singleSize * len(dst)
	gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, ssbo)
	ptr := gl.MapBufferRange(gl.SHADER_STORAGE_BUFFER, 0, bufSize, gl.MAP_READ_BIT)
	if ptr == nil {
		return glErrOrMessage("failed to map SSBO buffer during copy")
	}
	defer gl.UnmapBuffer(gl.SHADER_STORAGE_BUFFER)
	gpuBytes := unsafe.Slice((*byte)(ptr), bufSize)
	bufBytes := unsafe.Slice((*byte)(unsafe.Pointer(&dst[0])), bufSize)
	copy(bufBytes, gpuBytes)
	return nil
}

func computeEvaluate[T ms2.Vec | ms3.Vec](pos []T, dist []float32, invocX int, objects []glbuild.ShaderObject) (err error) {
	if len(pos) != len(dist) {
		return errors.New("positional and distance buffers not equal in length")
	} else if len(dist) == 0 {
		return errors.New("zero length buffers")
	} else if invocX < 1 {
		return errors.New("zero or negative invocation size")
	}

	var p runtime.Pinner
	var numSSBOs int
	for i := range objects {
		if objects[i].IsBindable() {
			numSSBOs++
		}
	}
	if numSSBOs > 0 {
		ssbosIDs := make([]uint32, numSSBOs)
		p.Pin(&ssbosIDs[0])
		gl.GenBuffers(int32(len(ssbosIDs)), &ssbosIDs[0])
		defer gl.DeleteBuffers(int32(len(ssbosIDs)), &ssbosIDs[0])

		iid := 0
		for i := range objects {
			ssbo := &objects[i]
			if ssbo.IsBindable() {
				id := ssbosIDs[iid]
				if id == 0 {
					p.Unpin()
					return glErrOrMessage("zero id for SSBO set by GL during compute binding")
				}
				iid++
				gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, id)
				gl.BufferData(gl.SHADER_STORAGE_BUFFER, ssbo.Size, ssbo.Data, gl.STATIC_DRAW)
				gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, uint32(ssbo.Binding), id)
			}
		}
	}

	var posSSBO, distSSBO uint32
	p.Pin(&posSSBO)
	p.Pin(&distSSBO)
	defer p.Unpin()

	posSSBO = loadSSBO(pos, 0, gl.STATIC_DRAW)
	if posSSBO == 0 {
		return glErrOrMessage("zero SSBO id set by GL during compute loading")
	}

	defer gl.DeleteBuffers(1, &posSSBO)

	distSSBO = createSSBO(elemSize[float32]()*len(dist), 1, gl.DYNAMIC_READ)
	if distSSBO == 0 {
		return glErrOrMessage("zero id SSBO creating distance buffer")
	}
	nWorkX := (len(dist) + invocX - 1) / invocX
	defer gl.DeleteBuffers(1, &distSSBO)
	gl.DispatchCompute(uint32(nWorkX), 1, 1)
	gl.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT)
	err = copySSBO(dist, distSSBO)
	if err != nil {
		return err
	}
	return glgl.Err()
}

func glErrOrMessage(defaultMsg string) (err error) {
	err = glgl.Err()
	if err == nil {
		err = errors.New(defaultMsg)
	} else {
		err = fmt.Errorf("%s: %w", defaultMsg, err)
	}
	return err
}
