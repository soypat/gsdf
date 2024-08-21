# gsdf
Offshoot from [this project](https://github.com/soypat/sdf/pull/13). 

![circle](https://github.com/user-attachments/assets/91c99f47-0c52-4cb1-83e7-452b03b69dff)
![iso-screw](https://github.com/user-attachments/assets/6bc987b9-d522-42a4-89df-71a20c3ae7ff)



## Features

- Heapless algorithms for everything. No usage of GC in happy path.
- Generate visualization for your parts as shaders.
- Heapless Octree triangle renderer. Is stupid fast.
- GPU and CPU implementations for all shapes and operations. CPU implementations are actually faster for simple parts.
    - Design your part using one API, switch between CPU and GPU after design.
- Extremely coherent API design.
- TinyGo supported for CPU evaluation :)

## Package layour/structure

- `gsdf`: Top level package defines exact SDFs primitives and operations for use on CPU or GPU workloads. Consumes `glbuild` interfaces and logic to build shaders.
- `glbuild`: Automatic shader generation interfaces and logic.
- `gleval`: SDF evaluation interfaces and facilities, both CPU and GPU bound.
- `glrender`: Triangle rendering logic which consumes gleval. STL generation.
- `forge`: Composed shape generation such as `threads` package for generating screw threads. Engineering applications.


## Part design - NPT Flange example.
This was converted from the [original example](https://github.com/soypat/sdf/blob/main/examples/npt-flange/flange.go). See [README](https://github.com/soypat/sdf/tree/main/examples) for images.


See working example under [examples](./examples/) directory. Run on GPU with `-gpu` flag: `go run ./examples/npt-flange -gpu`

Output and timings for
- CPU: 12th Gen Intel i5-12400F (12) @ 4.400GHz
- GPU: AMD ATI Radeon RX 6800

```sh
$ time go run ./examples/npt-flange/ -gpu
enabled GPU usage
SDF created in  5.253108ms evaluated sdf 13808829 times, rendered 219992 triangles in 847.606426ms wrote file in 29.16306ms

real    0m1,307s
user    0m0,753s
sys     0m0,284s

$ time go run ./examples/npt-flange/ 
SDF created in  229.895µs evaluated sdf 13808405 times, rendered 220064 triangles in 2.411541291s wrote file in 28.265793ms

real    0m2,744s
user    0m2,826s
sys     0m0,280s
```

The result of running the example are two files:
- `nptflange.glsl`: Visualization shader that can be copy pasted into [shadertoy](https://www.shadertoy.com/new) to visualize the part, or rendered within your editor with an extension such as the [Shader Toy Visual Studio Code extension](https://marketplace.visualstudio.com/items?itemName=stevensona.shader-toy).
- `nptflange.stl`: Triangle model file used in 3D printing software such as [Cura](https://ultimaker.com/software/ultimaker-cura/). Can be visualized online in sites such as [View STL](https://www.viewstl.com/).

Below is the 3D scene code. Omits rendering pipeline.
```go
	const (
		tlen             = 18. / 25.4
		internalDiameter = 1.5 / 2.
		flangeH          = 7. / 25.4
		flangeD          = 60. / 25.4
	)
	var (
		npt    threads.NPT
		flange glbuild.Shader3D
		err    error
	)
	err = npt.SetFromNominal(1.0 / 2.0)
	if err != nil {
		return nil, err
	}

	pipe, _ := threads.Nut(threads.NutParms{
		Thread: npt,
		Style:  threads.NutCircular,
	})

	// Base plate which goes bolted to joint.
	flange, _ = gsdf.NewCylinder(flangeD/2, flangeH, flangeH/8)

	// Join threaded section with flange.
	flange = gsdf.Translate(flange, 0, 0, -tlen/2)
	union := gsdf.SmoothUnion(pipe, flange, 0.2)

	// Make through-hole in flange bottom. Holes usually done at the end
	// to avoid smoothing effects covering up desired negative space.
	hole, _ := gsdf.NewCylinder(internalDiameter/2, 4*flangeH, 0)
	union = gsdf.Difference(union, hole)
	// Convert from imperial inches units to millimeter:
	union = gsdf.Scale(union, 25.4)
	renderSDF(union)
```

![array-triangles](https://github.com/user-attachments/assets/6a479889-2836-464c-b8ea-82109a5aad13)