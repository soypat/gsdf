float gsdfHexagon2D(vec2 p, float r) {
const vec3 k = vec3(-0.8660254038,0.5,0.577350269);
p = abs(p);
p -= 2.0*min(dot(k.xy,p),0.0)*k.xy;
p -= vec2(clamp(p.x, -k.z*r, k.z*r), r);
return length(p)*sign(p.y);
}