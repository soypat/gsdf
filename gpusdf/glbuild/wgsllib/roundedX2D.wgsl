fn gsdfRoundedX2D(p_in: vec2f, w: f32, r: f32) -> f32 {
    let p = abs(p_in);
    return length(p-min(p.x+p.y,w)*0.5) - r;
}
