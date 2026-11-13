fn gsdfOctagon2D(p_in: vec2f, r: f32) -> f32 {
    const k = vec3f(-0.9238795325, 0.3826834323, 0.4142135623);
    var p = abs(p_in);
    p -= 2.0*min(dot(vec2f(k.x,k.y),p),0.0)*vec2f(k.x,k.y);
    p -= 2.0*min(dot(vec2f(-k.x,k.y),p),0.0)*vec2f(-k.x,k.y);
    p -= vec2f(clamp(p.x, -k.z*r, k.z*r), r);
    return length(p)*sign(p.y);
}
