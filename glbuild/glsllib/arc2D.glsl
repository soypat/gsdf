float gsdfArc2D(vec2 p, float r, float t, float sinAngle, float cosAngle) {
vec2 sc = vec2(sinAngle,cosAngle);
p.x=abs(p.x);
return ((sc.y*p.x>sc.x*p.y) ? length(p-sc*r) : abs(length(p)-r))-t;
}