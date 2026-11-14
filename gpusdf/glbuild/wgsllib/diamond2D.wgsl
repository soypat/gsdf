fn gsdfDiamond2D(p_in: vec2f, x: f32, y: f32) -> f32 {
    let b = vec2f(x,y);
    let p = abs(p_in);
    let ndot = b.x*(b.x-2.0*p.x)-b.y*(b.y-2.0*p.y);
    let h = clamp(ndot/dot(b,b), -1.0, 1.0);
    let d = length(p-0.5*b*vec2f(1.0-h,1.0+h));
    return d * sign(p.x*b.y + p.y*b.x - b.x*b.y);
}
