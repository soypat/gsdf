float gsdfBoxFrame3D(vec3 p, float x, float y, float z, float thick) {
vec3 dims = vec3(x,y,z);
p = abs(p)-dims;
vec3 q = abs(p+thick)-thick;
return min(min(
      length(max(vec3(p.x,q.y,q.z),0.0))+min(max(p.x,max(q.y,q.z)),0.0),
      length(max(vec3(q.x,p.y,q.z),0.0))+min(max(q.x,max(p.y,q.z)),0.0)),
      length(max(vec3(q.x,q.y,p.z),0.0))+min(max(q.x,max(q.y,p.z)),0.0));
}