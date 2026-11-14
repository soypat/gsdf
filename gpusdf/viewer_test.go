//go:build !tinygo && cgo && !js

package gpusdf

import (
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/oliverbestmann/webgpu/wgpu"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
)

// TestViewerOffscreen renders the viewer to an offscreen texture and checks the shape is visible.
// Set GSDF_VIEWER_PNG to a filename to save the rendered image.
func TestViewerOffscreen(t *testing.T) {
	var dev Device
	err := dev.Configure(DeviceConfig{})
	if err != nil {
		t.Skip("no GPU available:", err)
	}
	defer dev.Release()
	var bld gsdf.Builder
	body := bld.NewBox(2, 1, 1, 0.1)
	hole := bld.NewCylinder(0.3, 2, 0)
	shape := bld.Difference(body, bld.Rotate(hole, 1.5707963, ms3.Vec{X: 1}))
	knob := bld.Translate(bld.NewSphere(0.45), 0.8, 0.5, 0)
	shape = bld.SmoothUnion(0.1, shape, knob)
	shape = bld.Scale(shape, 0.6)

	const width, height = 256, 192 // Bytes per row must be a multiple of 256 for texture copies.
	const format = wgpu.TextureFormatRGBA8Unorm
	var v viewer
	err = v.configurePipeline(&dev, shape, format)
	if err != nil {
		t.Fatal(err)
	}
	defer v.release()
	ds := &dev.state
	texture, err := ds.device.TryCreateTexture(&wgpu.TextureDescriptor{
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		Dimension:     wgpu.TextureDimension2D,
		Size:          wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		Format:        format,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer texture.Release()
	view, err := texture.TryCreateView(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Release()
	err = v.renderTo(view, width, height)
	if err != nil {
		t.Fatal(err)
	}
	const size = width * height * 4
	buf, err := ds.device.TryCreateBuffer(&wgpu.BufferDescriptor{Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst, Size: size})
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Release()
	enc, err := ds.device.TryCreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	err = enc.TryCopyTextureToBuffer(&wgpu.TexelCopyTextureInfo{Texture: texture},
		&wgpu.TexelCopyBufferInfo{Buffer: buf, Layout: wgpu.TexelCopyBufferLayout{BytesPerRow: width * 4, RowsPerImage: height}},
		&wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1})
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := enc.TryFinish(nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.queue.Submit(cmd)
	var status wgpu.MapAsyncStatus
	err = buf.TryMapAsync(wgpu.MapModeRead, 0, size, func(s wgpu.MapAsyncStatus) { status = s })
	if err != nil {
		t.Fatal(err)
	}
	ds.device.Poll(true, nil)
	if status != wgpu.MapAsyncStatusSuccess {
		t.Fatal("map failed", status)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, buf.GetMappedRange(0, size))
	buf.TryUnmap()

	if filename := os.Getenv("GSDF_VIEWER_PNG"); filename != "" {
		fp, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer fp.Close()
		png.Encode(fp, img)
	}
	center := img.RGBAAt(width/2, height/2)
	corner := img.RGBAAt(0, 0)
	if center.R+center.G+center.B == 0 {
		t.Error("expected shape at center of image, got black pixel")
	}
	if corner.R+corner.G+corner.B != 0 {
		t.Errorf("expected black background at corner, got %v", corner)
	}
}
