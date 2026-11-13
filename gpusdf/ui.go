package gpusdf

import (
	"context"
	"errors"
	"math"

	"github.com/soypat/gsdf/glbuild"
)

// UIConfig configures the interactive 3D shape visualizer. See [UI].
type UIConfig struct {
	// Width and Height of the window in pixels. If zero a default size is used.
	// Ignored on WebAssembly targets if CanvasID references an existing canvas.
	Width, Height int
	// Context cancels the UI when done. May be nil.
	Context context.Context
	// CanvasID is the HTML id of the canvas element the visualizer renders to on WebAssembly targets.
	// If empty or not found a new canvas is created and appended to the document body.
	CanvasID string
}

// UI opens an interactive visualizer that renders s on the GPU by raymarching.
// Drag with the left mouse button to orbit the camera around the shape and scroll to zoom.
// UI blocks until the window is closed or the context is done.
//
// On native targets a window is created with GLFW, which requires UI to be called from the main
// thread on some operating systems (macOS). Lock the main goroutine to the main thread on initialization:
//
//	func init() { runtime.LockOSThread() }
//
// On WebAssembly targets the shape is rendered to an HTML canvas; UI must not be called from a JavaScript callback.
func UI(s glbuild.Shader3D, cfg UIConfig) error {
	if s == nil {
		return errors.New("gpusdf: nil shader")
	}
	if cfg.Width <= 0 {
		cfg.Width = 800
	}
	if cfg.Height <= 0 {
		cfg.Height = 600
	}
	return ui(s, cfg)
}

// orbitCamera is the state of a camera orbiting the origin controlled by mouse input.
type orbitCamera struct {
	yaw, pitch       float64
	dist             float64 // distance from the origin.
	minDist, maxDist float64
	charDist         float64 // characteristic dimension of scene.
	lastX, lastY     float64
	dragging         bool
	firstMove        bool
}

func (c *orbitCamera) reset(sceneDiagonal float32) {
	diag := float64(sceneDiagonal)
	*c = orbitCamera{
		dist:     diag,
		minDist:  diag * 1e-5,
		maxDist:  diag * 10,
		charDist: diag,
	}
}

func (c *orbitCamera) press() {
	c.dragging = true
	c.firstMove = true
}

func (c *orbitCamera) release() { c.dragging = false }

// move updates the camera angles on a mouse position change. Returns true if camera changed.
func (c *orbitCamera) move(x, y float64) bool {
	if !c.dragging {
		return false
	}
	if c.firstMove {
		c.lastX, c.lastY = x, y
		c.firstMove = false
	}
	const sensitivity = 0.005
	c.yaw -= (x - c.lastX) * sensitivity
	c.pitch -= (y - c.lastY) * sensitivity
	const maxPitch = math.Pi/2 - 0.01
	c.pitch = max(-maxPitch, min(maxPitch, c.pitch))
	c.lastX, c.lastY = x, y
	return true
}

// scroll zooms the camera in (positive yoff) or out.
func (c *orbitCamera) scroll(yoff float64) {
	c.dist -= yoff * (c.dist*.1 + .01)
	c.dist = max(c.minDist, min(c.maxDist, c.dist))
}

// viewParams is the uniform buffer data of the viewer render pipeline. Must match gsdfViewParams in viewerWGSL.
type viewParams struct {
	resolution [2]float32
	yaw        float32
	pitch      float32
	camDist    float32
	tmax       float32
	aa         int32
	srgb       uint32
}

func (c *orbitCamera) params(width, height int, aa int, srgb bool) viewParams {
	var srgbFlag uint32
	if srgb {
		srgbFlag = 1
	}
	return viewParams{
		resolution: [2]float32{float32(width), float32(height)},
		yaw:        float32(c.yaw),
		pitch:      float32(c.pitch),
		camDist:    float32(c.dist),
		tmax:       float32(1.3 * (c.dist + c.charDist)),
		aa:         int32(aa),
		srgb:       srgbFlag,
	}
}

// viewerWGSL contains the render pipeline entry points of the viewer. Must be appended
// to the output of [glbuild.Programmer.WriteRaymarchSDF3].
const viewerWGSL = `
struct gsdfViewParams {
    resolution: vec2f,
    yaw: f32,
    pitch: f32,
    camDist: f32,
    tmax: f32,
    aa: i32,
    srgb: u32,
}

@group(0) @binding(0) var<uniform> gsdfView: gsdfViewParams;

@vertex
fn vs_main(@builtin(vertex_index) vi: u32) -> @builtin(position) vec4f {
    // Single triangle covering the entire screen.
    let uv = vec2f(f32((vi << 1u) & 2u), f32(vi & 2u));
    return vec4f(uv*2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn fs_main(@builtin(position) pos: vec4f) -> @location(0) vec4f {
    let fragCoord = vec2f(pos.x, gsdfView.resolution.y - pos.y);
    var col = gsdfRender(fragCoord, gsdfView.resolution, gsdfView.yaw, gsdfView.pitch, gsdfView.camDist, gsdfView.tmax, gsdfView.aa);
    if (gsdfView.srgb != 0u) {
        col = col*col; // Undo gamma correction since sRGB render target applies it.
    }
    return vec4f(col, 1.0);
}
`
