fn gsdfHexagon3D(p_in: vec3f, side: f32, extrude: f32) -> f32 {
    let h = vec2f(side, extrude);
    const k = vec3f(-0.8660254038, 0.5, 0.57735);
    var p = abs(p_in);
    let pxy = p.xy - 2.0*min(dot(k.xy, p.xy), 0.0)*k.xy;
    p = vec3f(pxy, p.z);
    let aux = p.xy-vec2f(clamp(p.x,-k.z*h.x,k.z*h.x), h.x);
    let d = vec2f(length(aux)*sign(p.y-h.x), p.z-h.y);
    return min(max(d.x,d.y),0.0) + length(max(d,vec2f(0.0)));
}
