float gsdfEqTri(vec2 p, float h) {
const float k = sqrt(3.0);
p.x = abs(p.x) - h;
p.y = p.y + h/k;
if( p.x+k*p.y>0.0 ) p = vec2(p.x-k*p.y,-k*p.x-p.y)/2.0;
p.x -= clamp( p.x, -2.0*h, 0.0 );
return -length(p)*sign(p.y);
}