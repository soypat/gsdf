fn gsdfTorus3D(p: vec3f, t1: f32, t2: f32) -> f32 {
    let t = vec2f(t1, t2);
    let q = vec2f(length(p.xz)-t.x,p.y);
    return length(q)-t.y;
}
