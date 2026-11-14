// Package main shows a shape rendered on the GPU via WebGPU. The same program runs natively,
// opening a window, and in the browser when compiled with GOOS=js GOARCH=wasm, rendering to a canvas.
// See index.html for instructions on running in the browser.
package main

import (
	"bytes"
	"log"
	"runtime"

	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gpusdf"
	"github.com/soypat/gsdf/gpusdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

func init() {
	runtime.LockOSThread() // UI window (GLFW) must run on main thread on some operating systems (macOS, linux).
}

func scene(bld *gsdf.Builder) glbuild.Shader3D {
	body := bld.NewBox(2, 1, 1, 0.1)
	hole := bld.NewCylinder(0.3, 2, 0)
	shape := bld.Difference(body, bld.Rotate(hole, 1.5707963, ms3.Vec{X: 1}))
	knob := bld.Translate(bld.NewSphere(0.45), 0.8, 0.5, 0)
	shape = bld.SmoothUnion(0.1, shape, knob)
	return bld.Scale(shape, 0.6)
}

func main() {
	var bld gsdf.Builder
	shape := scene(&bld)
	if err := bld.Err(); err != nil {
		log.Fatal("creating scene:", err)
	}
	err := compareGPUCPU(shape)
	if err != nil {
		log.Println("GPU compute check:", err)
	}
	err = gsdfaux.UI(shape, gsdfaux.UIConfig{
		Width:    800,
		Height:   600,
		CanvasID: "gsdf", // Used only in the browser.
	})
	if err != nil {
		log.Fatal("UI:", err)
	}
}

// compareGPUCPU evaluates the shape with WebGPU compute and on the CPU and logs the largest difference.
func compareGPUCPU(shape glbuild.Shader3D) error {
	var dev gpusdf.Device
	err := dev.Configure(gpusdf.DeviceConfig{})
	if err != nil {
		return err
	}
	defer dev.Release()
	prog := glbuild.NewDefaultProgrammer()
	wg := min(256, dev.MaxWorkgroupSize())
	prog.SetComputeInvocations(wg, 1, 1)
	var src bytes.Buffer
	_, objs, err := prog.WriteComputeSDF3(&src, shape)
	if err != nil {
		return err
	}
	var gpu gpusdf.SDF3
	defer gpu.Release()
	err = gpu.Configure(&dev, shape.Bounds(), gpusdf.ComputeConfig{Source: src.Bytes(), WorkgroupSize: wg, ShaderObjects: objs})
	if err != nil {
		return err
	}
	cpu, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		return err
	}
	pos := ms3.AppendGrid(nil, shape.Bounds(), 32, 32, 32)
	distGPU := make([]float32, len(pos))
	distCPU := make([]float32, len(pos))
	err = gpu.Evaluate(pos, distGPU, nil)
	if err != nil {
		return err
	}
	err = cpu.Evaluate(pos, distCPU, cpu.VecPool())
	if err != nil {
		return err
	}
	var maxDiff float32
	for i := range distGPU {
		maxDiff = max(maxDiff, abs(distGPU[i]-distCPU[i]))
	}
	log.Printf("evaluated %d positions on %s: max GPU/CPU difference %g", len(pos), dev.AdapterInfo(), maxDiff)
	return nil
}

func abs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
