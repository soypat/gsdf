//go:build !tinygo && cgo

package gsdf_test

import (
	"bytes"
	"log"
	"math"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/soypat/glgl/v4.6-core/glgl"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
)

// Since GPU must be run in main thread we need to do some dark arts for GPU code to be code-covered.
func TestMain(m *testing.M) {
	runtime.LockOSThread()
	var exit int
	cfg := newShaderTestConfig()
	err := testGsdfGPU(cfg)
	if err != nil {
		exit = 1
		log.Println(err)
	}
	if cfg.failedObj != nil {
		ui(cfg.failedObj, 800, 600)
	}
	runtime.UnlockOSThread()
	os.Exit(m.Run() | exit)
}

func testGsdfGPU(cfg *shaderTestConfig) error {
	term, err := gleval.Init1x1GLFW()
	if err != nil {
		log.Fatal(err)
	}
	defer term()
	invoc := glgl.MaxComputeInvocations()
	prog := glbuild.NewDefaultProgrammer()
	prog.SetComputeInvocations(invoc, 1, 1)
	cfg.useGPU = true
	err = testGSDF(cfg)
	return err
}

func ui(s glbuild.Shader3D, width, height int) error {
	bb := s.Bounds()
	// Initialize GLFW
	window, term, err := startGLFW(width, height)
	if err != nil {
		log.Fatal(err)
	}
	defer term()
	var sdfDecl bytes.Buffer
	programmer := glbuild.NewDefaultProgrammer()
	err = glbuild.ShortenNames3D(&s, 12)
	if err != nil {
		return err
	}

	root, _, _, err := programmer.WriteSDFDecl(&sdfDecl, s)
	if err != nil {
		return err
	}
	// Print OpenGL version
	// // Compile shaders and link program
	prog, err := glgl.CompileProgram(glgl.ShaderSource{
		Vertex: `#version 460
in vec2 aPos;
out vec2 vTexCoord;
void main() {
    vTexCoord = aPos * 0.5 + 0.5;
    gl_Position = vec4(aPos, 0.0, 1.0);
}
` + "\x00",
		Fragment: makeFragSource(root, sdfDecl.String()),
	})
	if err != nil {
		log.Fatal(err)
	}
	prog.Bind()
	// Define a quad covering the screen
	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	vertices := []float32{
		-1.0, -1.0,
		1.0, -1.0,
		-1.0, 1.0,
		-1.0, 1.0,
		1.0, -1.0,
		1.0, 1.0,
	}
	gl.BufferData(gl.ARRAY_BUFFER, 4*len(vertices), gl.Ptr(vertices), gl.STATIC_DRAW)
	camDistUniform, err := prog.UniformLocation("uCamDist\x00")
	if err != nil {
		return err
	}
	resUniform, err := prog.UniformLocation("uResolution\x00")
	if err != nil {
		return err
	}
	yawUniform, err := prog.UniformLocation("uYaw\x00") // gl.GetUniformLocation(program, gl.Str("uResolution\x00"))
	if err != nil {
		return err
	}
	pitchUniform, err := prog.UniformLocation("uPitch\x00")
	if err != nil {
		return err
	}
	// Specify the layout of the vertex data
	posAttrib, err := prog.AttribLocation("aPos\x00")
	if err != nil {
		return err
	}
	gl.EnableVertexAttribArray(posAttrib)
	gl.VertexAttribPointerWithOffset(posAttrib, 2, gl.FLOAT, false, 0, 0)
	// Enable depth testing
	gl.Enable(gl.DEPTH_TEST)

	// Set up mouse input tracking
	diag := bb.Diagonal()
	minZoom := float64(diag * 0.00001)
	maxZoom := float64(diag * 10)
	var (
		yaw              float64
		pitch            float64
		lastMouseX       float64
		lastMouseY       float64
		camDist          float64 = float64(diag) // initial camera distance
		firstMouseMove           = true
		isMousePressed           = false
		yawSensitivity           = 0.005
		pitchSensitivity         = 0.005
		refresh                  = true
	)

	window.SetCursorPosCallback(func(w *glfw.Window, xpos float64, ypos float64) {
		if !isMousePressed {
			return
		}
		refresh = true
		if firstMouseMove {
			lastMouseX = xpos
			lastMouseY = ypos
			firstMouseMove = false
		}

		deltaX := xpos - lastMouseX
		deltaY := ypos - lastMouseY

		yaw += deltaX * yawSensitivity
		pitch -= deltaY * pitchSensitivity // Invert y-axis

		// Clamp pitch
		pi := math.Pi
		maxPitch := pi/2 - 0.01
		if pitch > maxPitch {
			pitch = maxPitch
		}
		if pitch < -maxPitch {
			pitch = -maxPitch
		}

		lastMouseX = xpos
		lastMouseY = ypos
	})

	window.SetScrollCallback(func(w *glfw.Window, xoff, yoff float64) {
		refresh = true
		camDist -= yoff * (camDist*.1 + .01)
		if camDist < minZoom {
			camDist = minZoom // Minimum zoom level
		}
		if camDist > maxZoom {
			camDist = maxZoom // Maximum zoom level
		}
	})

	window.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		switch button {
		case glfw.MouseButtonLeft:
			refresh = true
			if action == glfw.Press {
				isMousePressed = true
				firstMouseMove = true
				window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
			} else if action == glfw.Release {
				isMousePressed = false
				window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
			}

		}
	})

	// Main render loop
	previousTime := glfw.GetTime()
	for !window.ShouldClose() {
		width, height := window.GetSize()
		currentTime := glfw.GetTime()
		elapsedTime := currentTime - previousTime
		previousTime = currentTime
		_ = elapsedTime
		// Clear the screen
		gl.ClearColor(0.0, 0.0, 0.0, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		// Set uniforms
		prog.Bind()
		// gl.UseProgram(program)
		gl.Uniform1f(camDistUniform, float32(camDist))
		gl.Uniform2f(resUniform, float32(width), float32(height))
		gl.Uniform1f(yawUniform, float32(yaw))
		gl.Uniform1f(pitchUniform, float32(pitch))

		// Draw the quad
		gl.BindVertexArray(vao)
		gl.DrawArrays(gl.TRIANGLES, 0, 6)
		// Swap buffers and poll events
		window.SwapBuffers()

		// Limit frame rate
		for {
			time.Sleep(time.Second / 60)
			glfw.PollEvents()
			if refresh || window.ShouldClose() {
				refresh = false
				break
			}
		}
	}
	return nil
}

func makeFragSource(rootSDFName, sdfDecl string) string {
	var buf bytes.Buffer

	buf.WriteString("#version 460\n")
	buf.WriteString(sdfDecl + "\n")
	// Function to calculate the SDF (Signed Distance Function)
	buf.WriteString("float sdf(vec3 p) {\n\treturn " + rootSDFName + "(p); \n};\n")
	buf.WriteString(`in vec2 vTexCoord;
out vec4 fragColor;


uniform vec2 uResolution;
uniform float uYaw;
uniform float uPitch;



// Function to calculate the normal at a point using central differences
vec3 calcNormal(vec3 pos) {
    const float eps = 0.0001;
    vec2 e = vec2(1.0, -1.0) * 0.5773;
    return normalize(
        e.xyy * sdf(pos + e.xyy * eps) +
        e.yyx * sdf(pos + e.yyx * eps) +
        e.yxy * sdf(pos + e.yxy * eps) +
        e.xxx * sdf(pos + e.xxx * eps)
    );
}

uniform float uCamDist; // Distance from the target. Controlled by mouse scroll (zoom).

void main() {
    vec2 fragCoord = vTexCoord * uResolution;

    // Constants
    const float PI = 3.14159265359;

    // Camera setup
    
    vec3 ta = vec3(0.0, 0.0, 0.0); // Camera target at the origin

    // Use accumulated yaw and pitch
    float yaw = uYaw;
    float pitch = uPitch;

    // Clamp pitch to prevent flipping
    pitch = clamp(pitch, -PI / 2.0 + 0.01, PI / 2.0 - 0.01);

    // Calculate camera direction
    vec3 dir;
    dir.x = cos(pitch) * sin(yaw);
    dir.y = sin(pitch);
    dir.z = cos(pitch) * cos(yaw);

    vec3 ro = ta - dir * uCamDist; // Camera position

    // Camera matrix
    vec3 ww = normalize(ta - ro);                        // Forward vector
    vec3 uu = normalize(cross(ww, vec3(0.0, 1.0, 0.0))); // Right vector
    vec3 vv = cross(uu, ww);                             // Up vector

    // Pixel coordinates
    vec2 p = (2.0 * fragCoord - uResolution) / uResolution.y;

    // Create view ray
    vec3 rd = normalize(p.x * uu + p.y * vv + 1.5 * ww);

    // Ray marching
    const float tmax = 100.0;
    float t = 0.0;
    bool hit = false;
    for (int i = 0; i < 256; i++) {
        vec3 pos = ro + t * rd;
        float h = sdf(pos);
        if (h < 0.0001 || t > tmax) {
            hit = true;
            break;
        }
        t += h;
    }

    // Shading/lighting
    vec3 col = vec3(0.0);
    if (hit) {
        vec3 pos = ro + t * rd;
        vec3 nor = calcNormal(pos);
        float dif = clamp(dot(nor, vec3(0.57703)), 0.0, 1.0);
        float amb = 0.5 + 0.5 * dot(nor, vec3(0.0, 1.0, 0.0));
        col = vec3(0.2, 0.3, 0.4) * amb + vec3(0.8, 0.7, 0.5) * dif;
    }

    // Gamma correction
    col = sqrt(col);

    fragColor = vec4(col, 1.0);
}
`)
	buf.WriteByte(0)
	return buf.String()
}

func startGLFW(width, height int) (window *glfw.Window, term func(), err error) {
	if err := glfw.Init(); err != nil {
		log.Fatalln("Failed to initialize GLFW:", err)
	}

	// Create GLFW window
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 6)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.Resizable, glfw.False)

	window, err = glfw.CreateWindow(width, height, "gsdf 3D Shape Visualizer", nil, nil)
	if err != nil {
		log.Fatalln("Failed to create GLFW window:", err)
	}
	window.MakeContextCurrent()

	// Initialize OpenGL
	if err := gl.Init(); err != nil {
		log.Fatalln("Failed to initialize OpenGL:", err)
	}
	return window, glfw.Terminate, err
}
