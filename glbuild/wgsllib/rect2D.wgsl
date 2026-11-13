fn gsdfRect2D(p: vec2f, x: f32, y: f32) -> f32 {
    let b = vec2f(x,y);
    let d = abs(p)-b;
    return length(max(d,vec2f(0.0))) + min(max(d.x,d.y),0.0);
}
