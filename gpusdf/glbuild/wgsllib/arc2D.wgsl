fn gsdfArc2D(p_in: vec2f, r: f32, t: f32, sinAngle: f32, cosAngle: f32) -> f32 {
    var p = p_in;
    let sc = vec2f(sinAngle, cosAngle);
    p.x = abs(p.x);
    return select(abs(length(p)-r), length(p-sc*r), sc.y*p.x > sc.x*p.y) - t;
}
