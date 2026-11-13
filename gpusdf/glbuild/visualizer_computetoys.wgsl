
// Entry point for https://compute.toys. Paste the entire file into the editor.
// Drag the mouse to orbit the camera around the shape.
@compute @workgroup_size(16, 16)
fn main_image(@builtin(global_invocation_id) id: vec3u) {
    let screen_size = textureDimensions(screen);
    if (id.x >= screen_size.x || id.y >= screen_size.y) {
        return;
    }
    const PI = 3.14159265359;
    let res = vec2f(screen_size);
    let fragCoord = vec2f(f32(id.x) + 0.5, res.y - f32(id.y) - 0.5);
    var m = vec2f(0.5, 0.5); // Default mouse position at center of screen.
    if (mouse.pos.x != 0 || mouse.pos.y != 0) {
        m = vec2f(mouse.pos) / res;
    }
    let yaw = (m.x - 0.5) * 2.0 * PI;
    let pitch = (0.5 - m.y) * PI;
    let col = gsdfRender(fragCoord, res, yaw, pitch, 3.0, 3.0, 2);
    textureStore(screen, id.xy, vec4f(col, 1.0));
}
