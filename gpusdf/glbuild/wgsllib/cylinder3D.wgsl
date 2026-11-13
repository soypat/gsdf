fn gsdfCylinder3D(p_in: vec3f, radius: f32, h: f32, rnd: f32) -> f32 {
    let p = p_in.xzy;
    let d = vec2f(length(p.xz)-radius+rnd, abs(p.y) - h);
    return min(max(d.x,d.y),0.0) + length(max(d,vec2f(0.0))) - rnd;
}
