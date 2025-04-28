vec4 gsdfPartialCircArray2D(vec2 p, float ncirc, float angle, float ninsm1) {
    float pangle=atan(p.y, p.x);
	float i=floor(pangle/angle);
	if (i<0.0) i=ncirc+i;
	float i0,i1;
	if (i>=ninsm1) {
		i0=ninsm1;
		i1=0.0;
	} else {
		i0=i;
		i1=i+1.0;
	}
	float c0 = cos(angle*i0);
	float s0 = sin(angle*i0);
	vec2 p0 = mat2(c0,-s0,s0,c0)*p;
	float c1 = cos(angle*i1);
	float s1 = sin(angle*i1);
	vec2 p1 = mat2(c1,-s1,s1,c1)*p;
    return vec4(p0.x,p0.y,p1.x,p1.y);
}