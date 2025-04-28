float gsdfOctagon2D(vec2 p, float r) {
const vec3 k = vec3(-0.9238795325, 0.3826834323, 0.4142135623 );
p = abs(p);
p -= 2.0*min(dot(vec2( k.x,k.y),p),0.0)*vec2( k.x,k.y);
p -= 2.0*min(dot(vec2(-k.x,k.y),p),0.0)*vec2(-k.x,k.y);
p -= vec2(clamp(p.x, -k.z*r, k.z*r), r);
return length(p)*sign(p.y);
}