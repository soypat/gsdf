float gsdfHexagon3D(vec3 p, float side, float extrude) {
vec2 h = vec2(side, extrude);
const vec3 k = vec3(-0.8660254038, 0.5, 0.57735);
p = abs(p);
p.xy -= 2.0*min(dot(k.xy, p.xy), 0.0)*k.xy;
vec2 aux = p.xy-vec2(clamp(p.x,-k.z*h.x,k.z*h.x), h.x);
vec2 d = vec2( length(aux)*sign(p.y-h.x), p.z-h.y );
return min(max(d.x,d.y),0.0) + length(max(d,0.0));
}