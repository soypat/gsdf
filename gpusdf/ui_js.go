//go:build !tinygo && js

package gpusdf

import (
	"syscall/js"
	"time"

	"github.com/oliverbestmann/webgpu/wgpu"
	"github.com/soypat/gsdf/glbuild"
)

func ui(s glbuild.Shader3D, cfg UIConfig) error {
	document := js.Global().Get("document")
	var canvas js.Value
	if cfg.CanvasID != "" {
		canvas = document.Call("getElementById", cfg.CanvasID)
	}
	if canvas.IsUndefined() || canvas.IsNull() {
		canvas = document.Call("createElement", "canvas")
		canvas.Set("width", cfg.Width)
		canvas.Set("height", cfg.Height)
		document.Get("body").Call("appendChild", canvas)
	}
	var dev Device
	err := dev.Configure(DeviceConfig{PowerPreference: PowerPreferenceHighPerformance})
	if err != nil {
		return err
	}
	defer dev.Release()
	surface := dev.state.instance.CreateSurface(&wgpu.SurfaceDescriptor{Canvas: canvas})
	defer surface.Release()
	var v viewer
	err = v.configure(&dev, surface, preferredCanvasFormat(), s, canvas.Get("width").Int(), canvas.Get("height").Int())
	if err != nil {
		return err
	}
	defer v.release()

	// JavaScript callbacks only modify camera state. All WebGPU calls are performed in the loop below
	// since blocking calls inside JavaScript callbacks deadlock the Go runtime.
	const aaIdle, aaInteract = 3, 1
	refresh := true
	lastEdit := time.Now()
	flagEdit := func() {
		refresh = true
		lastEdit = time.Now()
		v.aa = aaInteract
	}
	listeners := []struct {
		event string
		fn    js.Func
	}{
		{"pointerdown", js.FuncOf(func(this js.Value, args []js.Value) any {
			ev := args[0]
			if ev.Get("button").Int() == 0 {
				canvas.Call("setPointerCapture", ev.Get("pointerId"))
				v.cam.press()
				flagEdit()
			}
			return nil
		})},
		{"pointerup", js.FuncOf(func(this js.Value, args []js.Value) any {
			v.cam.release()
			flagEdit()
			return nil
		})},
		{"pointermove", js.FuncOf(func(this js.Value, args []js.Value) any {
			ev := args[0]
			if v.cam.move(ev.Get("clientX").Float(), ev.Get("clientY").Float()) {
				flagEdit()
			}
			return nil
		})},
		{"wheel", js.FuncOf(func(this js.Value, args []js.Value) any {
			ev := args[0]
			ev.Call("preventDefault")
			v.cam.scroll(-ev.Get("deltaY").Float() / 100)
			flagEdit()
			return nil
		})},
	}
	for _, l := range listeners {
		canvas.Call("addEventListener", l.event, l.fn)
	}
	defer func() {
		for _, l := range listeners {
			canvas.Call("removeEventListener", l.event, l.fn)
			l.fn.Release()
		}
	}()

	ctx := cfg.Context
	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if w, h := canvas.Get("width").Int(), canvas.Get("height").Int(); w != int(v.config.Width) || h != int(v.config.Height) {
			v.resize(w, h)
			refresh = true
		}
		if refresh {
			refresh = false
			err = v.draw()
			if err != nil {
				return err
			}
		}
		time.Sleep(time.Second / 60) // Yields to JavaScript event loop.
		if !v.cam.dragging && v.aa != aaIdle && time.Since(lastEdit) > 300*time.Millisecond {
			v.aa = aaIdle
			refresh = true
		}
	}
}

// preferredCanvasFormat returns the browser's preferred canvas format, avoiding an extra copy on presentation.
func preferredCanvasFormat() wgpu.TextureFormat {
	gpu := js.Global().Get("navigator").Get("gpu")
	if gpu.IsUndefined() {
		return 0
	}
	switch gpu.Call("getPreferredCanvasFormat").String() {
	case "rgba8unorm":
		return wgpu.TextureFormatRGBA8Unorm
	case "bgra8unorm":
		return wgpu.TextureFormatBGRA8Unorm
	}
	return 0
}
