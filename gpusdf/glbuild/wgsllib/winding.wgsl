fn gsdfWinding(p: vec2f, v1: vec2f, v2: vec2f, d_s_in: vec2f) -> vec2f {
    var d_s = d_s_in;
    let e = v2 - v1;
    let w = p - v1;
    let b = w - e*clamp(dot(w,e)/dot(e,e), 0.0, 1.0);
    d_s.x = min(d_s.x, dot(b,b));
    // winding number from http://geomalgorithms.com/a03-_inclusion.html
    // Comparisons are parenthesized to avoid WGSL template list ambiguity (a<b, c>d).
    let cond = vec3<bool>((p.y >= v1.y), (p.y < v2.y), (e.x*w.y > e.y*w.x));
    if (all(cond) || all(!cond)) {
        d_s.y = -d_s.y;
    }
    return d_s;
}
