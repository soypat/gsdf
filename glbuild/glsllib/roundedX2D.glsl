float gsdfRoundedX2D(vec2 p, float w, float r){
p = abs(p);
return length(p-min(p.x+p.y,w)*0.5) - r;
}