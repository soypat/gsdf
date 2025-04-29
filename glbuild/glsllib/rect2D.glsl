float gsdfRect2D(vec2 p, float x, float y) {
    vec2 b = vec2(x,y); 
    vec2 d = abs(p)-b;
	return length(max(d,0.0)) + min(max(d.x,d.y),0.0);
}