float gsdfLineSq2D(vec2 p, vec4 v1v2) {
	vec2 a = v1v2.xy;
	vec2 b = v1v2.zw;
	vec2 pa = p-a, ba = b-a;
	float h = clamp( dot(pa,ba)/dot(ba,ba), 0.0, 1.0 );
	vec2 dv = pa -ba*h;
	return dot(dv, dv);
}
