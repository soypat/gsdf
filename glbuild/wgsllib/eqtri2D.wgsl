fn gsdfEqTri(p_in: vec2f, h: f32) -> f32 {
    const k = 1.7320508075688772; // sqrt(3)
    var p = p_in;
    p.x = abs(p.x) - h;
    p.y = p.y + h/k;
    if (p.x+k*p.y > 0.0) {
        p = vec2f(p.x-k*p.y,-k*p.x-p.y)/2.0;
    }
    p.x -= clamp(p.x, -2.0*h, 0.0);
    return -length(p)*sign(p.y);
}
