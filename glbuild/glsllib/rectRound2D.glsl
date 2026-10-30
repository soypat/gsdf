float gsdfRectRound2D(vec2 p, float x, float y, float r) {
    vec2 b = vec2(x,y); 
    vec2 d = abs(p)-b + r;
	return length(max(d,0.0)) + min(max(d.x,d.y),0.0) - r;
}