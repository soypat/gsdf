# gsdf
Offshoot from [this project](https://github.com/soypat/sdf/pull/13). Is WIP.

## Features

- Heapless algorithms for everything. No usage of GC in happy path.
- Generate visualization for your parts as shaders.
- Heapless Octree triangle renderer. Is stupid fast.
- GPU and CPU implementations for all shapes and operations. CPU implementations are actually faster for simple parts.
    - Design your part using one API, switch between CPU and GPU after design.
- Extremely coherent API design.

## Package layour/structure

- `gsdf`: Top level package defines exact SDFs primitives and operations for use on CPU or GPU workloads. Consumes `glbuild` interfaces and logic to build shaders.
- `glbuild`: Automatic shader generation interfaces and logic.
- `gleval`: SDF evaluation interfaces and facilities, both CPU and GPU bound.
- `glrender`: Triangle rendering logic which consumes gleval. STL generation.
- `forge`: Composed shape generation such as `threads` package for generating screw threads. Engineering applications.

## Part design - NPT Flange example.
This was converted from the [original example](https://github.com/soypat/sdf/blob/main/examples/npt-flange/flange.go). See [README](https://github.com/soypat/sdf/tree/main/examples) for images.

```go
    const (
		tlen             = 18. / 25.4 // thread length
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
	pipe, err := threads.Nut(threads.NutParms{
		Thread: npt,
		Style:  threads.NutCircular,
	})
	if err != nil {
		return nil, err
	}
	flange, err = gsdf.NewCylinder(flangeD/2, flangeH, flangeH/8)
	if err != nil {
		return nil, err
	}
	flange = gsdf.Translate(flange, 0, 0, -tlen/2)
	flange = gsdf.SmoothUnion(pipe, flange, 0.2)
	hole, err := gsdf.NewCylinder(internalDiameter/2, 4*flangeH, 0)
	if err != nil {
		return nil, err
	}
	flange = gsdf.Difference(flange, hole) // Make through-hole in flange bottom
	flange = gsdf.Scale(flange, 25.4)      // convert to millimeters
	renderSDF(flange)
```