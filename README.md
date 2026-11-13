# gsdf

[![go.dev reference](https://pkg.go.dev/badge/github.com/soypat/gsdf)](https://pkg.go.dev/github.com/soypat/gsdf)
[![Go Report Card](https://goreportcard.com/badge/github.com/soypat/gsdf)](https://goreportcard.com/report/github.com/soypat/gsdf)
[![codecov](https://codecov.io/gh/soypat/gsdf/branch/main/graph/badge.svg)](https://codecov.io/gh/soypat/gsdf)
[![Go](https://github.com/soypat/gsdf/actions/workflows/go.yml/badge.svg)](https://github.com/soypat/gsdf/actions/workflows/go.yml)
[![sourcegraph](https://sourcegraph.com/github.com/soypat/gsdf/-/badge.svg)](https://sourcegraph.com/github.com/soypat/gsdf?badge)

`gsdf` is a CAD 3D design library for Go that uses SDFs for shape definition. Rendering can be done on GPU or CPU
for visualization or 3D printing file outputs. Quick jump to usage: [bolt example](./examples/bolt/main.go).

All images and shapes in readme were generated using this library.

![bolt-example](https://github.com/user-attachments/assets/8da50871-2415-423f-beb3-0d78ad67c79e)
![circle](https://github.com/user-attachments/assets/91c99f47-0c52-4cb1-83e7-452b03b69dff)
![text](https://github.com/user-attachments/assets/73a90941-9279-449d-9f4d-3f2746af5dd5)

## Requirements

- [Go](https://go.dev/) 1.25 or newer.
- **Optional, for GPU usage**: GPU evaluation and the UI use [WebGPU](https://github.com/oliverbestmann/webgpu) which runs on Metal (macOS), Vulkan (Linux, Windows), D3D12 (Windows) and in the browser (WASM).
    - Native builds require cgo and a GPU driver. On Debian/Ubuntu install `libegl1-mesa-dev mesa-vulkan-drivers libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev libxkbcommon-dev libwayland-dev`.
    - The first cgo build of a program importing `gpusdf` (or `gsdfaux`) downloads prebuilt [wgpu-native](https://github.com/gfx-rs/wgpu-native) static libraries for all operating systems (several hundred MB). CPU-only users of `gsdf`, `gleval` and `glrender` do not download them.
    - Builds without cgo and TinyGo builds compile and run on CPU; GPU functions return an error.

## Features

- High test coverage. GPU tests run when a GPU is available, including software GPUs (lavapipe) in CI.

- **NEW!**: GPU evaluation via WebGPU. Runs on macOS, Linux, Windows and in the browser. See [`gpusdf`](./gpusdf) and the [WASM viewer example](./examples/wasm-viewer).

- Reusable WGSL code between shaders. See [`gpusdf/glbuild/wgsllib`](gpusdf/glbuild/wgsllib)

- Extremely coherent API design.

- UI for visualizing parts, rendered directly from shaders. See [UI example](./examples/ui-mandala) by running `go run ./examples/ui-mandala`. Same UI runs in the browser.

- Generate visualization for your parts as WGSL shaders that can be pasted into [compute.toys](https://compute.toys).

- GPU and CPU implementations for all shapes and operations. CPU implementations are actually faster for simple parts.

- Include arbitrary buffers into GPU calculation. See [`Shader` interface](./gpusdf/glbuild/glbuild.go).

- Heapless algorithms for everything. No usage of GC in happy path.

- Heapless Octree triangle renderer. Is stupid fast.
    - Design your part using one API, switch between CPU and GPU after design.

- TinyGo supported for CPU evaluation :)

## Package layout/structure

- `gsdf`: Top level package defines exact SDFs primitives and operations for use on CPU or GPU workloads. Consumes `glbuild` interfaces and logic to build shaders.
- `gpusdf/glbuild`: Shape interfaces and automatic WGSL shader generation. Does not depend on WebGPU, importing it does not import `gpusdf`.
- `gleval`: SDF evaluation interfaces and CPU evaluation facilities.
- `gpusdf`: GPU evaluation of SDFs and the interactive UI using WebGPU. Compiles on all targets; functions return an error where no GPU support is available.
- `glrender`: Triangle rendering logic which consumes gleval. STL generation.
- `forge`: Engineering applications. Composed of subpackages.
    - `textsdf` package for text generation.
    - `threads` package for generating screw threads.
- `gsdfaux`: High level helper functions to get users started up with `gsdf`. See [examples](./examples).


# Examples
Find examples under [examples](./examples/) directory. Run on GPU with: `-gpu` flag.

Most 3D examples output two files:
- `example-name.wgsl`: Visualization shader that can be copy pasted into [compute.toys](https://compute.toys) to visualize the part.
- `example-name.stl`: Triangle model file used in 3D printing software such as [Cura](https://ultimaker.com/software/ultimaker-cura/). Can be visualized online in sites such as [View STL](https://www.viewstl.com/).


CPU output and timings on 12th Gen Intel i5-12400F (12) @ 4.400GHz. GPU timings on AMD ATI Radeon RX 6800 (Vulkan).

The [WASM viewer example](./examples/wasm-viewer) runs the same program natively or in the browser, see its `index.html` for instructions:
```sh
GOOS=js GOARCH=wasm go build -o examples/wasm-viewer/main.wasm ./examples/wasm-viewer
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" examples/wasm-viewer/
python3 -m http.server -d examples/wasm-viewer 8080
```

### Migrating from OpenGL
gsdf previously used OpenGL 4.3 compute shaders. GPU evaluation now uses WebGPU:
- `glbuild` moved to `github.com/soypat/gsdf/gpusdf/glbuild` (and `glsllib` to `gpusdf/glbuild/wgsllib`). Update imports, the package name is unchanged.
- Shaders are generated in WGSL. `Shader.AppendShaderBody` was renamed to `AppendWGSLBody` so custom [`glbuild.Shader`](./gpusdf/glbuild/glbuild.go) implementations fail to compile until ported from GLSL to WGSL.
- `gleval.NewComputeGPUSDF3/2`, `gleval.Init1x1GLFW`, `PolygonGPU`, `Lines2DGPU`, `DisplaceMulti2D` and `Batcher` moved to `gpusdf` (`gpusdf.SDF3`, `gpusdf.Device`, `gpusdf.Polygon`, ...).
- `gsdfaux.MakeGPUSDF2` receives a `*gpusdf.Device`.
- `runtime.LockOSThread` is no longer required for GPU evaluation. It is still required for the UI on macOS.

## `simplesdf` Python-like API
The simplesdf package provides a extremely simplified API for use by makers who want a more Python-like API. This is particularily useful for short one-off scripts. Note this API uses panics to help track down errors and also is not thread-safe. See [`examples/simple-knurled-cylinder`](./examples/simple-knurled-cylinder/).
```go
package main

import (
	"math"

	. "github.com/soypat/gsdf/gsdfaux/simplesdf"
)

func main() {
	// main body
	f := Cylinder(1, 5, 0.1)

	// knurling
	x := Box(1, 1, 4, 0).RotateZ(math.Pi / 4)
	x = x.Translate(1.6, 0, 0)              // radial placement (fogleman circular_array offset)
	x = x.CircArray(24, 24)                 // 24 instances evenly around the circle
	x = x.Twist(0.75).Union(x.Twist(-0.75)) // diamond pattern via mirrored twist
	f = f.Diff(x.K(0.1))

	// central hole
	f = f.Diff(Cylinder(0.5, 7, 0).K(0.1))

	// vent holes
	c := Cylinder(0.25, 3, 0).RotateY(math.Pi / 2) // orient along X
	f = f.Diff(c.Translate(0, 0, -2.5).K(0.1))
	f = f.Diff(c.Translate(0, 0, 2.5).K(0.1))

	f.Save("knurling.stl", STLConfig{ResolutionDivisions: 200})
}
```

## npt-flange
This was converted from the [original sdf library example](https://github.com/soypat/sdf/blob/main/examples/npt-flange/flange.go).

#### GPU rendering in 2.8 seconds. 0.4M triangles
```sh
go run ./examples/npt-flange -resdiv 400 -gpu
[-] using GPU	ᵍᵒᵗᵗᵃ ᵍᵒ ᶠᵃˢᵗ
[144ms] init GPU radv AMD Radeon RX 6800 (RADV NAVI21) (vulkan, discrete-gpu) with compute invocation size 64
[14.67ms] GPU shader generated and compiled
[158ms] instantiating evaluation SDF took
[223.6µs] wrote nptflange.wgsl
[2.48s] evaluated SDF 46148745 times and rendered 423852 triangles with 95.7 percent evaluations omitted in octree pruning step with resolution 0.21679485
[194ms] wrote nptflange.stl
[2.83s] render done
finished npt-flange example
```

#### CPU rendering in 0.5 seconds. 0.4M triangles
```sh
go run ./examples/npt-flange -resdiv 400
[-] using CPU
[13.2µs] instantiating evaluation SDF took
[-] CPU parallel evaluation on 11 goroutines
[153.4µs] wrote nptflange.wgsl
[322ms] evaluated SDF 6711686 times and rendered 423852 triangles with resolution 0.21679485
[136ms] wrote nptflange.stl
[459ms] render done
finished npt-flange example
```

![npt-flange-example](https://github.com/user-attachments/assets/32a00926-0a1e-47f0-8b6c-dda940240265)


### fibonacci-showerhead

Note that the amount of triangles is very similar to the NPT flange example, but this part is more computationally complex.

#### GPU rendering in 1.3 seconds. 0.3M triangles
```sh
go run ./examples/fibonacci-showerhead -resdiv 350 -gpu
[-] using GPU	ᵍᵒᵗᵗᵃ ᵍᵒ ᶠᵃˢᵗ
[32.7ms] init GPU radv AMD Radeon RX 6800 (RADV NAVI21) (vulkan, discrete-gpu) with compute invocation size 64
[56.21ms] GPU shader generated and compiled
[88.93ms] instantiating evaluation SDF took
[186.3µs] wrote showerhead.wgsl
[918ms] evaluated SDF 14646431 times and rendered 309872 triangles with 89.08 percent evaluations omitted in octree pruning step with resolution 0.2979682
[255ms] wrote showerhead.stl
[1.26s] render done
showerhead example done
```

#### CPU rendering in 1.2 seconds. 0.3M triangles
```sh
go run ./examples/fibonacci-showerhead -resdiv 350
[-] using CPU
[23.6µs] instantiating evaluation SDF took
[-] CPU parallel evaluation on 11 goroutines
[612.9µs] wrote showerhead.wgsl
[1.01s] evaluated SDF 1512025 times and rendered 309872 triangles with resolution 0.2979682
[182ms] wrote showerhead.stl
[1.19s] render done
showerhead example done
```

![fibonacci-showerhead](https://github.com/user-attachments/assets/a72c366c-6ee0-43ba-9128-087a76524ff9)

## More examples

![iso-screw](https://github.com/user-attachments/assets/6bc987b9-d522-42a4-89df-71a20c3ae7ff)
![array-triangles](https://github.com/user-attachments/assets/6a479889-2836-464c-b8ea-82109a5aad13)
![geb-book-cover](https://github.com/user-attachments/assets/a6727481-07f3-4636-8e1c-9b1a02bb108f)