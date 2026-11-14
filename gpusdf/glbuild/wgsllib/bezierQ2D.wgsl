fn gsdfBezierQ2D(p: vec2f, A: vec2f, B: vec2f, C: vec2f, thick: f32) -> f32 {
    let a = B - A;
    let b = A + C - 2.0*B;
    let c = a * 2.0;
    let d = A - p;
    let kk = 1.0/dot(b,b);
    let kx = kk * dot(a,b);
    let ky = kk * (2.0*dot(a,a)+dot(d,b))/3.0;
    let kz = kk * dot(d,a);
    var res = 0.0;
    var sgn = 0.0;
    let g = ky - kx*kx;
    let q = kx*(2.0*kx*kx - 3.0*ky) + kz;
    let g3 = g*g*g;
    let q2 = q*q;
    var h = q2 + 4.0*g3;
    if (h >= 0.0) {
        h = sqrt(h);
        var x = (vec2f(h,-h)-q)/2.0;
        if (abs(g) < 0.001) {
            let k = (1.0-g3/q2)*g3/q;
            x = vec2f(k,-k-q);
        }
        let uv = sign(x)*pow(abs(x), vec2f(1.0/3.0));
        var t = uv.x + uv.y;
        t -= (t*(t*t+3.0*g)+q)/(3.0*t*t+3.0*g);
        t = clamp(t-kx, 0.0, 1.0);
        let w = d+(c+b*t)*t;
        res = dot(w,w);
        let aux = c+2.0*b*t;
        sgn = aux.x*w.y-aux.y*w.x;
    } else {
        let z = sqrt(-g);
        let aux = q/(g*z*2.0);
        let x = sqrt(0.5+0.5*aux);
        let m = x*(x*(x*(x*-0.008972+0.039071)-0.107074)+0.576975)+0.5;
        var n = sqrt(1.0-m*m);
        n *= sqrt(3.0);
        let t = clamp(vec3f(m+m,-n-m,n-m)*z-kx, vec3f(0.0), vec3f(1.0));
        var aux2 = a+b*t.x;
        let qx = d+(c+b*t.x)*t.x;
        let dx = dot(qx,qx);
        let sx = aux2.x*qx.y - aux2.y*qx.x;
        aux2 = a+b*t.y;
        let qy = d+(c+b*t.y)*t.y;
        let dy = dot(qy,qy);
        let sy = aux2.x*qy.y - aux2.y*qy.x;
        if (dx < dy) {
            res = dx;
            sgn = sx;
        } else {
            res = dy;
            sgn = sy;
        }
    }
    return sqrt(res) - thick;
}
