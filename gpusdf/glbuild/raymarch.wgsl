// Raymarching library. Requires a `fn sdf(p: vec3f) -> f32` function to be defined.
//
// Derived from work by Inigo Quilez under the MIT License:
// The MIT License
// Copyright © 2023 Inigo Quilez
// Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions: The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software. THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// gsdfCalcNormal calculates the SDF normal at pos using the tetrahedron technique.
// https://iquilezles.org/articles/normalsSDF
fn gsdfCalcNormal(pos: vec3f, eps: f32) -> vec3f {
    let e = vec2f(1.0, -1.0) * 0.5773;
    return normalize(e.xyy*sdf(pos + e.xyy*eps) +
                     e.yyx*sdf(pos + e.yyx*eps) +
                     e.yxy*sdf(pos + e.yxy*eps) +
                     e.xxx*sdf(pos + e.xxx*eps));
}

// gsdfRender returns the gamma corrected color of the pixel at fragCoord (origin at bottom-left of image)
// for a camera orbiting the origin at a distance camDist. Rays are marched up to a distance of tmax.
// aa is the antialiasing level, aa*aa rays are cast per pixel.
fn gsdfRender(fragCoord: vec2f, resolution: vec2f, yaw: f32, pitchIn: f32, camDist: f32, tmax: f32, aa: i32) -> vec3f {
    const PI = 3.14159265359;
    let pitch = clamp(pitchIn, -PI/2.0 + 0.01, PI/2.0 - 0.01); // Prevent gimbal lock.
    // Camera setup.
    let ta = vec3f(0.0, 0.0, 0.0); // Camera target at the origin.
    let dir = vec3f(cos(pitch)*sin(yaw), sin(pitch), cos(pitch)*cos(yaw));
    let ro = ta - dir*camDist; // Camera position.
    // Camera matrix.
    let ww = normalize(ta - ro);                         // Forward vector.
    let uu = normalize(cross(ww, vec3f(0.0, 1.0, 0.0))); // Right vector.
    let vv = cross(uu, ww);                              // Up vector.
    let n = max(aa, 1);
    var tot = vec3f(0.0);
    for (var m = 0; m < n; m++) {
        for (var k = 0; k < n; k++) {
            let o = vec2f(f32(m), f32(k)) / f32(n) - 0.5;
            let p = (2.0*(fragCoord + o) - resolution) / resolution.y;
            let rd = normalize(p.x*uu + p.y*vv + 1.5*ww); // View ray.
            // Ray marching.
            const tol = 0.0001;
            var t = 0.0;
            var hit = false;
            for (var i = 0; i < 256; i++) {
                let h = sdf(ro + t*rd);
                if (h < tol) {
                    hit = true;
                    break;
                }
                if (t > tmax) {
                    break;
                }
                t += h;
            }
            // Shading/lighting.
            if (hit) {
                let nor = gsdfCalcNormal(ro + t*rd, tol);
                let dif = clamp(dot(nor, vec3f(0.57703)), 0.0, 1.0);
                let amb = 0.5 + 0.5*dot(nor, vec3f(0.0, 1.0, 0.0));
                tot += sqrt(vec3f(0.2, 0.3, 0.4)*amb + vec3f(0.8, 0.7, 0.5)*dif); // Gamma corrected.
            }
        }
    }
    return tot / f32(n*n);
}
