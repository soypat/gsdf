# gsdf
Offshoot from [this project](https://github.com/soypat/sdf/pull/13). Is WIP.

## Features

- Heapless algorithms for everything. No usage of GC in happy path.
- Generate visualization for your parts as shaders.
- Heapless Octree triangle renderer. Is stupid fast.
- GPU and CPU implementations for all shapes and operations. CPU implementations are actually faster for simple parts.
    - Design your part using one API, switch between CPU and GPU after design.
- Extremely coherent API design.

## NPT Flange example.
This was converted from the [original example](https://github.com/soypat/sdf/blob/main/examples/npt-flange/flange.go). See [README](https://github.com/soypat/sdf/tree/main/examples) for images.
```go
var (
    npt    threads.NPT
    flange glbuild.Shader3D
)
npt.SetFromNominal(1.0 / 2.0)
pipe, err := threads.Nut(threads.NutParms{
    Thread: npt,
    Style:  threads.NutCircular,
})
if err != nil {
    panic(err)
}

flange, err = gsdf.NewCylinder(flangeD/2, flangeH, flangeH/8)
return makeSDF(flange)
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

render(flange) // Do something with it.
```