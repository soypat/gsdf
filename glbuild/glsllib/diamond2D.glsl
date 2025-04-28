float gsdfDiamond2D(vec2 p, vec2 b){
p = abs(p);
float ndot = b.x*(b.x-2.*p.x)-b.y*(b.y-2*p.y);
float h = clamp( ndot/dot(b,b), -1.0, 1.0 );
float d = length( p-0.5*b*vec2(1.0-h,1.0+h) );
return d * sign( p.x*b.y + p.y*b.x - b.x*b.y );
}