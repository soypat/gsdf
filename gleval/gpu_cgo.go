//go:build !tinygo && cgo

package gleval

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf/glbuild"
)

func (lines *Lines2DGPU) evaluate(pos []ms2.Vec, dist []float32, userData any) (err error) {
	if len(pos) != len(dist) {
		return errors.New("position and distance buffer length mismatch")
	} else if lines.shader == "" {
		return errors.New("need to initialize LinesGPU before first use")
	}
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: lines.shader})
	if err != nil {
		return fmt.Errorf("compiling GL program: %w", err)
	}
	defer prog.Delete()
	prog.Bind()
	prog.SetUniform1f("WidthOffset\x00", lines.Width/2)
	err = glgl.Err()
	if err != nil {
		return fmt.Errorf("binding LinesGPU program: %w", err)
	}
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadSSBO(lines.Lines, 2, gl.STATIC_DRAW)
	err = glgl.Err()
	if err != nil {
		return err
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
	} else if poly.shader == "" {
		return errors.New("need to initialize PolygonGPU before first use")
	}
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: poly.shader})
	if err != nil {
		return fmt.Errorf("compiling GL program: %w", err)
	}
	defer prog.Delete()
	prog.Bind()
	err = glgl.Err()
	if err != nil {
		return fmt.Errorf("binding PolygonGPU program: %w", err)
	}
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadSSBO(poly.Vertices, 2, gl.STATIC_DRAW)
	err = glgl.Err()
	if err != nil {
		return err
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
	} else if len(lines.shader) == 0 {
		return errors.New("need to initialize LinesGPU before first use")
	}
	cmp := unsafe.String(&lines.shader[0], len(lines.shader))
	prog, err := glgl.CompileProgram(glgl.ShaderSource{Compute: cmp})
	if err != nil {
		return fmt.Errorf("compiling GL program: %w", err)
	}
	defer prog.Delete()
	prog.Bind()

	err = glgl.Err()
	if err != nil {
		return fmt.Errorf("binding LinesGPU program: %w", err)
	}
	defer prog.Unbind()
	var p runtime.Pinner
	ssbo := loadSSBO(lines.Displacements, 2, gl.STATIC_DRAW)
	err = glgl.Err()
	if err != nil {
		return err
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
		err := glgl.Err()
		if err != nil {
			return err
		}
		return errors.New("failed to map buffer")
	}
	defer gl.UnmapBuffer(gl.SHADER_STORAGE_BUFFER)
	gpuBytes := unsafe.Slice((*byte)(ptr), bufSize)
	bufBytes := unsafe.Slice((*byte)(unsafe.Pointer(&dst[0])), bufSize)
	copy(bufBytes, gpuBytes)
	return glgl.Err()
}

func computeEvaluate[T ms2.Vec | ms3.Vec](pos []T, dist []float32, invocX int, ssbos []glbuild.ShaderObject) (err error) {
	if len(pos) != len(dist) {
		return errors.New("positional and distance buffers not equal in length")
	} else if len(dist) == 0 {
		return errors.New("zero length buffers")
	} else if invocX < 1 {
		return errors.New("zero or negative invocation size")
	}

	var p runtime.Pinner
	if len(ssbos) > 0 {
		ssbosIDs := make([]uint32, len(ssbos))
		p.Pin(&ssbosIDs[0])
		gl.GenBuffers(int32(len(ssbosIDs)), &ssbosIDs[0])
		defer gl.DeleteBuffers(int32(len(ssbosIDs)), &ssbosIDs[0])
		for i, id := range ssbosIDs {
			ssbo := ssbos[i]
			gl.BindBuffer(gl.SHADER_STORAGE_BUFFER, id)
			gl.BufferData(gl.SHADER_STORAGE_BUFFER, ssbo.Size, ssbo.Data, gl.STATIC_DRAW)
			gl.BindBufferBase(gl.SHADER_STORAGE_BUFFER, uint32(ssbo.Binding), id)
		}
		err := glgl.Err()
		if err != nil {
			return fmt.Errorf("binding SSBOs: %w", err)
		}
	}

	var posSSBO, distSSBO uint32
	p.Pin(&posSSBO)
	p.Pin(&distSSBO)
	defer p.Unpin()

	posSSBO = loadSSBO(pos, 0, gl.STATIC_DRAW)
	err = glgl.Err()
	if err != nil {
		return err
	}
	defer gl.DeleteBuffers(1, &posSSBO)

	distSSBO = createSSBO(elemSize[float32]()*len(dist), 1, gl.DYNAMIC_READ)
	err = glgl.Err()
	if err != nil {
		return err
	}
	nWorkX := (len(dist) + invocX - 1) / invocX
	defer gl.DeleteBuffers(1, &distSSBO)
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
	err = copySSBO(dist, distSSBO)
	if err != nil {
		return err
	}
	return nil
}
