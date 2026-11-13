fn gsdfBox3D(p: vec3f, x: f32, y: f32, z: f32, rnd: f32) -> f32 {
    let dims = vec3f(x,y,z);
    let q = abs(p)-dims+rnd;
    return length(max(q,vec3f(0.0))) + min(max(q.x,max(q.y,q.z)),0.0)-rnd;
}
