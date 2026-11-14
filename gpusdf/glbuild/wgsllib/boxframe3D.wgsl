fn gsdfBoxFrame3D(p_in: vec3f, x: f32, y: f32, z: f32, thick: f32) -> f32 {
    let dims = vec3f(x,y,z);
    let p = abs(p_in)-dims;
    let q = abs(p+thick)-thick;
    return min(min(
        length(max(vec3f(p.x,q.y,q.z),vec3f(0.0)))+min(max(p.x,max(q.y,q.z)),0.0),
        length(max(vec3f(q.x,p.y,q.z),vec3f(0.0)))+min(max(q.x,max(p.y,q.z)),0.0)),
        length(max(vec3f(q.x,q.y,p.z),vec3f(0.0)))+min(max(q.x,max(q.y,p.z)),0.0));
}
