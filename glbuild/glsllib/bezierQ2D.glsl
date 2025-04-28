float gsdfBezierQ2D(vec2 p, vec2 A, vec2 B, vec2 C, float thick){
vec2 a = B - A;
vec2 b = A + C - 2.0*B;
vec2 c = a * 2.0;
vec2 d = A - p;
float kk = 1.0/dot(b,b);
float kx = kk * dot(a,b);
float ky = kk * (2.0*dot(a,a)+dot(d,b))/3.0;
float kz = kk * dot(d,a);
float res = 0.0;
float sgn = 0.0;
float g  = ky - kx*kx;
float q  = kx*(2.0*kx*kx - 3.0*ky) + kz;
float g3 = g*g*g;
float q2 = q*q;
float h  = q2 + 4.0*g3;
if ( h>=0.0 ) 
{
    h = sqrt(h);
    vec2 x = (vec2(h,-h)-q)/2.0;
    if ( abs(g)<0.001 ) 
    {
        float k = (1.0-g3/q2)*g3/q;
        x = vec2(k,-k-q);
    }
    vec2 uv = sign(x)*pow(abs(x), vec2(1.0/3.0));
    float t = uv.x + uv.y;
    t -= (t*(t*t+3.0*g)+q)/(3.0*t*t+3.0*g);
    t = clamp( t-kx, 0.0, 1.0 );
    vec2  w = d+(c+b*t)*t;
    res = dot(w,w);
    vec2 aux = c+2.0*b*t;
    sgn = aux.x*w.y-aux.y*w.x;
} else {
    float z = sqrt(-g);
    float aux = q/(g*z*2.0);
    float x = sqrt(0.5+0.5*aux);
    float m = x*(x*(x*(x*-0.008972+0.039071)-0.107074)+0.576975)+0.5;
    float n = sqrt(1.0-m*m);
    n *= sqrt(3.0);
    vec3  t = clamp( vec3(m+m,-n-m,n-m)*z-kx, 0.0, 1.0 );
    vec2 aux2 = a+b*t.x;
    vec2  qx=d+(c+b*t.x)*t.x; float dx=dot(qx,qx), sx=aux2.x*qx.y - aux2.y*qx.x;
    aux2 = a+b*t.y;
    vec2  qy=d+(c+b*t.y)*t.y; float dy=dot(qy,qy), sy=aux2.x*qy.y - aux2.y*qy.x;
    if( dx<dy ) {res=dx;sgn=sx;} else {res=dy;sgn=sy;}
}
return sqrt( res ) - thick;
}