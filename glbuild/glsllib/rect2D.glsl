float gsdfRect2D(vec2 p, vec2 b) {
    vec2 d = abs(p)-b;
	return length(max(d,0.0)) + min(max(d.x,d.y),0.0);
}