//go:build !tinygo && cgo && !js

package gpusdf

import (
	"fmt"
	"time"

	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/oliverbestmann/webgpu/wgpuglfw"
	"github.com/soypat/gsdf/glbuild"
)

func ui(s glbuild.Shader3D, cfg UIConfig) error {
	err := glfw.Init()
	if err != nil {
		return fmt.Errorf("gpusdf: initializing GLFW: %w", err)
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI) // WebGPU manages the surface.
	window, err := glfw.CreateWindow(cfg.Width, cfg.Height, "gsdf 3D Shape Visualizer", nil, nil)
	if err != nil {
		return fmt.Errorf("gpusdf: creating window: %w", err)
	}
	defer window.Destroy()

	var dev Device
	err = dev.Configure(DeviceConfig{PowerPreference: PowerPreferenceHighPerformance})
	if err != nil {
		return err
	}
	defer dev.Release()
	surface := dev.state.instance.CreateSurface(wgpuglfw.GetSurfaceDescriptor(window))
	defer surface.Release()
	var v viewer
	width, height := window.GetFramebufferSize()
	err = v.configure(&dev, surface, 0, s, width, height)
	if err != nil {
		return err
	}
	defer v.release()

	// Antialiasing is reduced while interacting for responsiveness.
	const aaIdle, aaInteract = 3, 1
	refresh := true
	lastEdit := time.Now()
	flagEdit := func() {
		refresh = true
		lastEdit = time.Now()
		v.aa = aaInteract
	}
	window.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		v.resize(width, height)
		flagEdit()
	})
	window.SetCursorPosCallback(func(_ *glfw.Window, xpos, ypos float64) {
		if v.cam.move(xpos, ypos) {
			flagEdit()
		}
	})
	window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		v.cam.scroll(yoff)
		flagEdit()
	})
	window.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if button != glfw.MouseButtonLeft {
			return
		}
		flagEdit()
		switch action {
		case glfw.Press:
			v.cam.press()
			w.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		case glfw.Release:
			v.cam.release()
			w.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
		}
	})

	ctx := cfg.Context
	for !window.ShouldClose() {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if refresh {
			refresh = false
			err = v.draw()
			if err != nil {
				return err
			}
		}
		time.Sleep(time.Second / 60)
		glfw.PollEvents()
		if !v.cam.dragging && v.aa != aaIdle && time.Since(lastEdit) > 300*time.Millisecond {
			v.aa = aaIdle // Render high quality frame after interaction stops.
			refresh = true
		}
	}
	return nil
}
