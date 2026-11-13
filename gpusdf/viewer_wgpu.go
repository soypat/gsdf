//go:build !tinygo && (cgo || js)

package gpusdf

import (
	"bytes"
	"errors"
	"fmt"
	"unsafe"

	"github.com/oliverbestmann/webgpu/wgpu"
	"github.com/soypat/gsdf/glbuild"
)

// viewer renders a raymarched SDF to a surface.
type viewer struct {
	dev      *Device
	surface  *wgpu.Surface
	config   wgpu.SurfaceConfiguration
	pipeline *wgpu.RenderPipeline
	uniform  *wgpu.Buffer
	bg       [2]*wgpu.BindGroup
	pools    []*wgpu.Buffer
	srgb     bool
	cam      orbitCamera
	aa       int
}

// configure configures the surface for rendering and compiles the render pipeline for s.
// preferred is the preferred surface format, if zero a format is selected automatically.
func (v *viewer) configure(dev *Device, surface *wgpu.Surface, preferred wgpu.TextureFormat, s glbuild.Shader3D, width, height int) (err error) {
	v.release()
	ds := &dev.state
	if ds.device == nil {
		return errDeviceNotReady
	}
	caps := surface.GetCapabilities(ds.adapter)
	if len(caps.Formats) == 0 {
		return errors.New("gpusdf: surface not supported by GPU adapter")
	}
	// Prefer non-sRGB formats since the raymarcher applies gamma correction.
	format := caps.Formats[0]
	for _, f := range caps.Formats {
		if f == preferred || preferred == 0 && (f == wgpu.TextureFormatBGRA8Unorm || f == wgpu.TextureFormatRGBA8Unorm) {
			format = f
			break
		}
	}
	err = v.configurePipeline(dev, s, format)
	if err != nil {
		return err
	}
	v.config = wgpu.SurfaceConfiguration{
		Usage:       wgpu.TextureUsageRenderAttachment,
		Format:      format,
		Width:       uint32(width),
		Height:      uint32(height),
		PresentMode: wgpu.PresentModeFifo,
		AlphaMode:   caps.AlphaModes[0],
	}
	surface.Configure(ds.device, &v.config)
	v.surface = surface
	return nil
}

// configurePipeline compiles the render pipeline for s rendering to a target of the given format.
func (v *viewer) configurePipeline(dev *Device, s glbuild.Shader3D, format wgpu.TextureFormat) (err error) {
	v.release()
	defer func() {
		if err != nil {
			v.release()
		}
	}()
	ds := &dev.state
	if ds.device == nil {
		return errDeviceNotReady
	}
	v.srgb = format == wgpu.TextureFormatBGRA8UnormSrgb || format == wgpu.TextureFormatRGBA8UnormSrgb
	err = glbuild.ShortenNames3D(&s, 12)
	if err != nil {
		return err
	}
	var src bytes.Buffer
	_, objs, err := glbuild.NewDefaultProgrammer().WriteRaymarchSDF3(&src, s)
	if err != nil {
		return err
	}
	src.WriteString(viewerWGSL)
	module, err := ds.device.TryCreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label:      "gsdf viewer",
		WGSLSource: &wgpu.ShaderSourceWGSL{Code: src.String()},
	})
	if err != nil {
		return fmt.Errorf("gpusdf: compiling viewer shader: %w", err)
	}
	defer module.Release()
	v.pipeline, err = ds.device.TryCreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "gsdf viewer",
		Vertex: wgpu.VertexState{
			Module:     module,
			EntryPoint: "vs_main",
		},
		Primitive: wgpu.PrimitiveState{
			Topology:  wgpu.PrimitiveTopologyTriangleList,
			FrontFace: wgpu.FrontFaceCCW,
			CullMode:  wgpu.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{
			Count: 1,
			Mask:  0xFFFFFFFF,
		},
		Fragment: &wgpu.FragmentState{
			Module:     module,
			EntryPoint: "fs_main",
			Targets: []wgpu.ColorTargetState{{
				Format:    format,
				Blend:     &wgpu.BlendStateReplace,
				WriteMask: wgpu.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("gpusdf: creating viewer pipeline: %w", err)
	}
	v.uniform, err = ds.device.TryCreateBuffer(&wgpu.BufferDescriptor{
		Label: "gsdf viewer params",
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		Size:  uint64(unsafe.Sizeof(viewParams{})),
	})
	if err != nil {
		return err
	}
	layout := v.pipeline.GetBindGroupLayout(0)
	defer layout.Release()
	v.bg[0], err = ds.device.TryCreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "gsdf viewer params",
		Layout:  layout,
		Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: v.uniform, Size: wgpu.WholeSize}},
	})
	if err != nil {
		return err
	}
	v.bg[1], v.pools, err = createPoolBindGroup(dev, v.pipeline, objs, v.pools[:0])
	if err != nil {
		return err
	}
	v.dev = dev
	v.cam.reset(s.Bounds().Diagonal())
	v.aa = 3
	return nil
}

func (v *viewer) resize(width, height int) {
	if width <= 0 || height <= 0 || v.surface == nil {
		return
	}
	v.config.Width = uint32(width)
	v.config.Height = uint32(height)
	v.surface.Configure(v.dev.state.device, &v.config)
}

// draw renders the current camera view to the surface.
func (v *viewer) draw() error {
	surfaceTexture, err := v.surface.TryGetCurrentTexture()
	if err != nil {
		return err
	}
	texture, ok := surfaceTexture.Get()
	if !ok {
		return nil // Surface may be occluded or outdated, skip frame.
	}
	defer texture.Release()
	view, err := texture.TryCreateView(nil)
	if err != nil {
		return err
	}
	defer view.Release()
	err = v.renderTo(view, int(v.config.Width), int(v.config.Height))
	if err != nil {
		return err
	}
	v.surface.Present()
	return nil
}

// renderTo renders the current camera view to a texture view of the configured format and submits the work.
func (v *viewer) renderTo(view *wgpu.TextureView, width, height int) error {
	ds := &v.dev.state
	params := v.cam.params(width, height, v.aa, v.srgb)
	err := ds.queue.TryWriteBuffer(v.uniform, 0, unsafe.Slice((*byte)(unsafe.Pointer(&params)), unsafe.Sizeof(params)))
	if err != nil {
		return err
	}
	encoder, err := ds.device.TryCreateCommandEncoder(nil)
	if err != nil {
		return err
	}
	defer encoder.Release()
	pass := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     wgpu.LoadOpClear,
			StoreOp:    wgpu.StoreOpStore,
			ClearValue: wgpu.Color{A: 1},
		}},
	})
	pass.SetPipeline(v.pipeline)
	pass.SetBindGroup(0, v.bg[0], nil)
	if v.bg[1] != nil {
		pass.SetBindGroup(glbuild.ObjectsBindGroup, v.bg[1], nil)
	}
	pass.Draw(3, 1, 0, 0)
	err = pass.TryEnd()
	if err != nil {
		return err
	}
	cmd, err := encoder.TryFinish(nil)
	if err != nil {
		return err
	}
	defer cmd.Release()
	ds.queue.Submit(cmd)
	return nil
}

func (v *viewer) release() {
	for _, b := range v.bg {
		if b != nil {
			b.Release()
		}
	}
	for i, b := range v.pools {
		b.Release()
		v.pools[i] = nil
	}
	if v.uniform != nil {
		v.uniform.Release()
	}
	if v.pipeline != nil {
		v.pipeline.Release()
	}
	pools := v.pools[:0]
	*v = viewer{pools: pools}
}
