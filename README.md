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

- [Go](https://go.dev/)
- **Optional**: See latest requirements on [go-glfw](https://github.com/go-gl/glfw) if using GPU

## Features

- High test coverage (when GPU available, not the case in CI)

- **NEW!**: Reusable GLSL code between shaders. See [`glbuild/glsllib`](glbuild/glsllib)

- Extremely coherent API design.

- UI for visualizing parts, rendered directly from shaders. See [UI example](./examples/ui-mandala) by running `go run ./examples/ui-mandala`

- Generate visualization for your parts as shaders.

- GPU and CPU implementations for all shapes and operations. CPU implementations are actually faster for simple parts.

- Include arbitrary buffers into GPU calculation. See [`Shader` interface](./glbuild/glbuild.go).

- Heapless algorithms for everything. No usage of GC in happy path.

- Heapless Octree triangle renderer. Is stupid fast.
    - Design your part using one API, switch between CPU and GPU after design.

- TinyGo supported for CPU evaluation :)

## Package layout/structure

- `gsdf`: Top level package defines exact SDFs primitives and operations for use on CPU or GPU workloads. Consumes `glbuild` interfaces and logic to build shaders.
- `glbuild`: Automatic shader generation interfaces and logic.
- `gleval`: SDF evaluation interfaces and facilities, both CPU and GPU bound.
- `glrender`: Triangle rendering logic which consumes gleval. STL generation.
- `forge`: Engineering applications. Composed of subpackages.
    - `textsdf` package for text generation.
    - `threads` package for generating screw threads.
- `gsdfaux`: High level helper functions to get users started up with `gsdf`. See [examples](./examples).


# Examples
Find examples under [examples](./examples/) directory. Run on GPU with: `-gpu` flag.

Most 3D examples output two files:
- `example-name.glsl`: Visualization shader that can be copy pasted into [shadertoy](https://www.shadertoy.com/new) to visualize the part, or rendered within your editor with an extension such as the [Shader Toy Visual Studio Code extension](https://marketplace.visualstudio.com/items?itemName=stevensona.shader-toy).
- `example-name.stl`: Triangle model file used in 3D printing software such as [Cura](https://ultimaker.com/software/ultimaker-cura/). Can be visualized online in sites such as [View STL](https://www.viewstl.com/).


CPU output and timings on 12th Gen Intel i5-12400F (12) @ 4.400GHz. GPU timings on AMD ATI Radeon RX 6800 (may be outdated).

## `simplesdf` Python-like API
The simplesdf package provides a extremely simplified API for use by makers who want a more Python-like API. This is particularily useful for short one-off scripts. Note this API uses panics to help track down errors and also is not thread-safe. See [`examples/simple-knurled-cylinder`](./examples/simple-knurled-cylinder/).
```go
package main

import (
	"math"
	"runtime"

	. "github.com/soypat/gsdf/gsdfaux/simplesdf"
)

func init() { runtime.LockOSThread() } // Required if using GPU to render shapes or using UI.

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

#### GPU rendering in 1.1 seconds. 0.4M triangles
```sh
 time go run ./examples/npt-flange -resdiv 400 -gpu
[-] using GPU   ᵍᵒᵗᵗᵃ ᵍᵒ ᶠᵃˢᵗ
[53.54ms] init GL with compute invocation size  1024
[7.73ms] GPU shader generated and compiled
[61.3ms] instantiating evaluation SDF took
[109.4µs] wrote nptflange.glsl
[706ms] evaluated SDF 46148745 times and rendered 423852 triangles with 95.7 percent evaluations omitted in octree pruning step with resolution 0.21679485
[371ms] wrote nptflange.stl
[1.14s] render done
finished npt-flange example
go run ./examples/npt-flange -resdiv 400 -gpu  0,75s user 0,49s system 102% cpu 1,219 total
```

#### CPU rendering in 0.7 seconds. 0.4M triangles
```sh
time go run ./examples/npt-flange -resdiv 400
[-] using CPU
[29.7µs] instantiating evaluation SDF took
[-] CPU parallel evaluation on 11 goroutines
[101.6µs] wrote nptflange.glsl
[313ms] evaluated SDF 6711686 times and rendered 423852 triangles with resolution 0.21679485
[341ms] wrote nptflange.stl
[654ms] render done
finished npt-flange example
go run ./examples/npt-flange -resdiv 400  2,78s user 0,38s system 422% cpu 0,747 total
```

![npt-flange-example](https://github.com/user-attachments/assets/32a00926-0a1e-47f0-8b6c-dda940240265)


### fibonacci-showerhead

Note that the amount of triangles is very similar to the NPT flange example, but this part is more computationally complex.

#### GPU rendering in 0.7 seconds. 0.3M triangles
```sh
time go run ./examples/fibonacci-showerhead -resdiv 350 -gpu
[-] using GPU   ᵍᵒᵗᵗᵃ ᵍᵒ ᶠᵃˢᵗ
[47.1ms] init GL with compute invocation size  1024
[62.9ms] GPU shader generated and compiled
[110ms] instantiating evaluation SDF took
[539.3µs] wrote showerhead.glsl
[337ms] evaluated SDF 14646431 times and rendered 309872 triangles with 89.08 percent evaluations omitted in octree pruning step with resolution 0.2979682
[252ms] wrote showerhead.stl
[701ms] render done
showerhead example done
go run ./examples/fibonacci-showerhead -resdiv 350 -gpu  0,60s user 0,36s system 118% cpu 0,814 total
```

#### CPU rendering in 1.3 seconds. 0.3M triangles
```sh
time go run ./examples/fibonacci-showerhead -resdiv 350
[-] using CPU
[34µs] instantiating evaluation SDF took
[-] CPU parallel evaluation on 11 goroutines
[326.4µs] wrote showerhead.glsl
[909ms] evaluated SDF 1512025 times and rendered 309872 triangles with resolution 0.2979682
[255ms] wrote showerhead.stl
[1.16s] render done
showerhead example done
go run ./examples/fibonacci-showerhead -resdiv 350  8,00s user 0,29s system 663% cpu 1,248 total
```

![fibonacci-showerhead](https://github.com/user-attachments/assets/a72c366c-6ee0-43ba-9128-087a76524ff9)

## More examples

![iso-screw](https://github.com/user-attachments/assets/6bc987b9-d522-42a4-89df-71a20c3ae7ff)
![array-triangles](https://github.com/user-attachments/assets/6a479889-2836-464c-b8ea-82109a5aad13)
![geb-book-cover](https://github.com/user-attachments/assets/a6727481-07f3-4636-8e1c-9b1a02bb108f)