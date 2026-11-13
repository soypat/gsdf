fn gsdfLineSq2D(p: vec2f, v1v2: vec4f) -> f32 {
    let a = v1v2.xy;
    let b = v1v2.zw;
    let pa = p-a;
    let ba = b-a;
    let h = clamp(dot(pa,ba)/dot(ba,ba), 0.0, 1.0);
    let dv = pa - ba*h;
    return dot(dv, dv);
}
