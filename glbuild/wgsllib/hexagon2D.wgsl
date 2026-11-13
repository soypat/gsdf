fn gsdfHexagon2D(p_in: vec2f, r: f32) -> f32 {
    const k = vec3f(-0.8660254038,0.5,0.577350269);
    var p = abs(p_in);
    p -= 2.0*min(dot(k.xy,p),0.0)*k.xy;
    p -= vec2f(clamp(p.x, -k.z*r, k.z*r), r);
    return length(p)*sign(p.y);
}
