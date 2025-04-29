float gsdfTorus3D(vec3 p, float t1, float t2) {
vec2 t = vec2(t1, t2);
vec2 q = vec2(length(p.xz)-t.x,p.y);
return length(q)-t.y;
}