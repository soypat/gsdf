vec2 gsdfWinding(vec2 p, vec2 v1, vec2 v2, vec2 d_s) {
    vec2 e = v2 - v1;
	vec2 w = p - v1;
	vec2 b = w - e*clamp( dot(w,e)/dot(e,e), 0.0, 1.0 );
	d_s.x = min( d_s.x, dot(b,b) );
	// winding number from http://geomalgorithms.com/a03-_inclusion.html
	bvec3 cond = bvec3( p.y>=v1.y, 
						p.y <v2.y, 
						e.x*w.y>e.y*w.x );
	if ( all(cond) || all(not(cond)) ) {
        d_s.y=-d_s.y;
    }
    return d_s;
}