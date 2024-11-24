# gsdf

[![go.dev reference](https://pkg.go.dev/badge/github.com/soypat/gsdf)](https://pkg.go.dev/github.com/soypat/gsdf)
[![Go Report Card](https://goreportcard.com/badge/github.com/soypat/gsdf)](https://goreportcard.com/report/github.com/soypat/gsdf)
[![codecov](https://codecov.io/gh/soypat/gsdf/branch/main/graph/badge.svg)](https://codecov.io/gh/soypat/gsdf)
[![Go](https://github.com/soypat/gsdf/actions/workflows/go.yml/badge.svg)](https://github.com/soypat/gsdf/actions/workflows/go.yml)
[![sourcegraph](https://sourcegraph.com/github.com/soypat/gsdf/-/badge.svg)](https://sourcegraph.com/github.com/soypat/gsdf?badge)

`gsdf` is a CAD 3D design library for Go that uses SDFs for shape definition. Rendering can be done on GPU or CPU
for visualization or 3D printing file outputs. Quick jump to usage: [bolt example](./examples/bolt/main.go).

All images and shapes in readme were generated using this library.

![circle](https://github.com/user-attachments/assets/91c99f47-0c52-4cb1-83e7-452b03b69dff)
![bolt-example](https://github.com/user-attachments/assets/8da50871-2415-423f-beb3-0d78ad67c79e)

## Requirements
- [Go](https://go.dev/)
- **Optional**: See latest requirements on [go-glfw](https://github.com/go-gl/glfw) if using GPU

## Features

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


Output and timings for
- CPU: 12th Gen Intel i5-12400F (12) @ 4.400GHz
- GPU: AMD ATI Radeon RX 6800

## npt-flange - 9× GPU speedup
This was converted from the [original sdf library example](https://github.com/soypat/sdf/blob/main/examples/npt-flange/flange.go).

#### GPU rendering in 1 second. 0.4M triangles
```sh
time go run ./examples/npt-flange -resdiv 400 -gpu
using GPU       ᵍᵒᵗᵗᵃ ᵍᵒ ᶠᵃˢᵗ
compute invocation size  1024
instantiating evaluation SDF took 115.587024ms
wrote nptflange.glsl in 97.829µs
evaluated SDF 46148621 times and rendered 423852 triangles in 1.103100086s with 95.7 percent evaluations omitted
wrote nptflange.stl in 710.038498ms
finished npt-flange example
go run ./examples/npt-flange -resdiv 400 -gpu  1,01s user 1,10s system 95% cpu 2,217 total
```

#### CPU rendering in 9 seconds. 0.4M triangles
```sh
time go run ./examples/npt-flange -resdiv 400 
using CPU
instantiating evaluation SDF took 14.173µs
wrote nptflange.glsl in 73.155µs
evaluated SDF 46147934 times and rendered 423852 triangles in 8.482344469s with 95.7 percent evaluations omitted
wrote nptflange.stl in 703.931017ms
finished npt-flange example
go run ./examples/npt-flange -resdiv 400  9,01s user 0,82s system 103% cpu 9,481 total
```

![npt-flange-example](https://github.com/user-attachments/assets/32a00926-0a1e-47f0-8b6c-dda940240265)


### fibonacci-showerhead - 40× GPU speedup

Note that the amount of triangles is very similar to the NPT flange example, but the speedup is much more notable due to the complexity of the part.

#### GPU rendering in 0.87 seconds. 0.3M triangles
```sh
time go run ./examples/fibonacci-showerhead -resdiv 350 -gpu
using GPU       ᵍᵒᵗᵗᵃ ᵍᵒ ᶠᵃˢᵗ
compute invocation size  1024
instantiating evaluation SDF took 108.241558ms
wrote showerhead.glsl in 581.351µs
evaluated SDF 14646305 times and rendered 309872 triangles in 768.731027ms with 89.08 percent evaluations omitted
wrote showerhead.stl in 509.470328ms
showerhead example done
go run ./examples/fibonacci-showerhead -resdiv 350 -gpu  0,87s user 0,69s system 94% cpu 1,646 total
```

#### CPU rendering in 36 seconds. 0.3M triangles
```sh
time go run ./examples/fibonacci-showerhead -resdiv 350 
using CPU
instantiating evaluation SDF took 27.757µs
wrote showerhead.glsl in 507.155µs
evaluated SDF 14645989 times and rendered 309872 triangles in 35.794768353s with 89.08 percent evaluations omitted
wrote showerhead.stl in 499.13903ms
SDF caching omitted 21.62 percent of 14645989 SDF evaluations
showerhead example done
go run ./examples/fibonacci-showerhead -resdiv 350  36,16s user 0,76s system 100% cpu 36,591 total
```

![fibonacci-showerhead](https://github.com/user-attachments/assets/a72c366c-6ee0-43ba-9128-087a76524ff9)

## More examples

![iso-screw](https://github.com/user-attachments/assets/6bc987b9-d522-42a4-89df-71a20c3ae7ff)
![array-triangles](https://github.com/user-attachments/assets/6a479889-2836-464c-b8ea-82109a5aad13)
![geb-book-cover](https://github.com/user-attachments/assets/1ed945fb-5729-4028-bed8-26e0de3073ab)