fn gsdfPartialCircArray2D(p: vec2f, ncirc: f32, angle: f32, ninsm1: f32) -> vec4f {
    // atan2(0,0) is undefined in WGSL (NaN on Metal). Use 0 to match CPU (Go's Atan2).
    let pangle = select(atan2(p.y, p.x), 0.0, all(p == vec2f(0.0)));
    var i = floor(pangle/angle);
    if (i < 0.0) {
        i = ncirc + i;
    }
    var i0: f32;
    var i1: f32;
    if (i >= ninsm1) {
        i0 = ninsm1;
        i1 = 0.0;
    } else {
        i0 = i;
        i1 = i + 1.0;
    }
    let c0 = cos(angle*i0);
    let s0 = sin(angle*i0);
    let p0 = mat2x2f(c0,-s0,s0,c0)*p;
    let c1 = cos(angle*i1);
    let s1 = sin(angle*i1);
    let p1 = mat2x2f(c1,-s1,s1,c1)*p;
    return vec4f(p0.x,p0.y,p1.x,p1.y);
}
