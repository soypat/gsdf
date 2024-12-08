package gsdfaux

import (
	"image/color"

	math "github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms1"
	"github.com/soypat/glgl/math/ms3"
)

// A great portion of logic in this file taken from Esme Lamb's (@dedelala)
// excellent color manipulation work presented at Gophercon AU 2024.
// https://github.com/dedelala/disco/tree/main/color

var red = color.RGBA{R: 255, A: 255}

// ColorConversionInigoQuilez creates a new color conversion using [Inigo Quilez]'s style.
// A good value for characteristic distance is the bounding box diagonal divided by 3. Returns red for NaN values.
//
// [Inigo Quilez]: https://iquilezles.org/articles/distfunctions2d/
func ColorConversionInigoQuilez(characteristicDistance float32) func(float32) color.Color {
	inv := 1. / characteristicDistance
	return func(d float32) color.Color {
		if math.IsNaN(d) {
			return red
		}
		d *= inv
		var one = ms3.Vec{X: 1, Y: 1, Z: 1}
		var c ms3.Vec
		if d > 0 {
			c = ms3.Vec{X: 0.9, Y: 0.6, Z: 0.3}
		} else {
			c = ms3.Vec{X: 0.65, Y: 0.85, Z: 1.0}
		}
		c = ms3.Scale(1-math.Exp(-6*math.Abs(d)), c)
		c = ms3.Scale(0.8+0.2*math.Cos(150*d), c)
		max := 1 - ms1.SmoothStep(0, 0.01, math.Abs(d))
		c = ms3.InterpElem(c, one, ms3.Vec{X: max, Y: max, Z: max})
		return color.RGBA{
			R: uint8(c.X * 255),
			G: uint8(c.Y * 255),
			B: uint8(c.Z * 255),
			A: 255,
		}
	}
}

// ColorConversionLinearGradient creates a color conversion function that creates a gradient centered
// along d=0 that extends gradientLength.
func ColorConversionLinearGradient(gradientLength float32, c0, c1 color.Color) func(d float32) color.Color {
	if c0 == color.Black && c1 == color.White {
		return blackAndWhiteLinearSmooth(gradientLength)
	}
	h0, s0, v0 := colorToHSV(c0)
	h1, s1, v1 := colorToHSV(c1)
	return func(d float32) color.Color {
		// Smoothstep anti-aliasing near the edge
		blend := d/gradientLength + 0.5
		if blend <= 0 {
			return c0
		} else if blend >= 1 {
			return c1
		}
		// Clamp blend to [0, 1] for colors in gradient range.
		h, s, v := interpHSV(h0, s0, v0, h1, s1, v1, blend)
		r, g, b := hsvToRGB(h, s, v)
		c := rgbToC(r, g, b)
		// Convert blend to gradient.
		return color.RGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 255}
	}
}

func blackAndWhiteLinearSmooth(edgeSmooth float32) func(d float32) color.Color {
	if edgeSmooth == 0 {
		return blackAndWhiteNoSmoothing
	}
	return func(d float32) color.Color {
		// Smoothstep anti-aliasing near the edge
		blend := d/edgeSmooth + 0.5
		// blend := 0.5 + 0.5*math32.Tanh(x)
		if blend <= 0 {
			return color.Black
		} else if blend >= 1 {
			return color.White
		}
		// Clamp blend to [0, 1] for valid grayscale values
		blend = ms1.Clamp(blend, 0, 1)
		// Convert blend to grayscale
		grayValue := uint8(blend * 255)
		return color.Gray{Y: grayValue}
	}
}

func blackAndWhiteNoSmoothing(d float32) color.Color {
	if d < 0 {
		return color.Black
	}
	return color.White
}

func percentUint64(num, denom uint64) float32 {
	return math.Trunc(10000*float32(num)/float32(denom)) / 100
}

func cInterp(c0, c1 uint32, t float32) uint32 {
	h0, s0, v0 := rgbToHSV(cToRGB(c0))
	h1, s1, v1 := rgbToHSV(cToRGB(c1))
	return rgbToC(hsvToRGB(interpHSV(h0, s0, v0, h1, s1, v1, t)))
}

func interpHSV(h0, s0, v0, h1, s1, v1, t float32) (h, s, v float32) {
	switch {
	case h1-h0 > 0.5:
		h0 += 1.0
	case h1-h0 < -0.5:
		h1 += 1.0
	}
	h = ms1.Interp(h0, h1, t)
	s = ms1.Interp(s0, s1, t)
	v = ms1.Interp(v0, v1, t)
	return h, s, v
}

func colorToHSV(c color.Color) (h, s, v float32) {
	r0, g0, b0, _ := c.RGBA()
	return rgbToHSV(float32(r0>>8)/math.MaxUint8, float32(g0>>8)/math.MaxUint8, float32(b0>>8)/math.MaxUint8)
}

// cToRGB converts a 24 bit RGB value stored in the least significant bits
func cToRGB(c uint32) (r, g, b float32) {
	r = float32(uint8(c>>16)) / math.MaxUint8
	g = float32(uint8(c>>8)) / math.MaxUint8
	b = float32(uint8(c)) / math.MaxUint8
	return r, g, b
}

// rgbToC converts r, g, and b float64 values on the range of 0.0 to 1.0 to a
// 24 bit RGB value stored in the least significant bits of a uint32. The inputs
// are clamped to the range of 0.0 to 1.0
func rgbToC(r, g, b float32) (c uint32) {
	return uint32(ms1.Clamp(r, 0, 1)*math.MaxUint8)<<16 |
		uint32(ms1.Clamp(g, 0, 1)*math.MaxUint8)<<8 |
		uint32(ms1.Clamp(b, 0, 1)*math.MaxUint8)
}

// hsvToRGB converts hue, saturation and brightness values on the range of 0.0
// to 1.0 to RGB floating point values on the range of 0.0 to 1.0
func hsvToRGB(h, s, v float32) (r, g, b float32) {
	var (
		c = s * v
		x = c * (1 - math.Abs(math.Mod(h*6, 2)-1))
		m = v - c
	)

	switch {
	case h >= 0 && h <= 1.0/6:
		r, g, b = c, x, 0
	case h > 1.0/6 && h <= 2.0/6:
		r, g, b = x, c, 0
	case h > 2.0/6 && h <= 3.0/6:
		r, g, b = 0, c, x
	case h > 3.0/6 && h <= 4.0/6:
		r, g, b = 0, x, c
	case h > 4.0/6 && h <= 5.0/6:
		r, g, b = x, 0, c
	case h > 5.0/6 && h <= 1.0:
		r, g, b = c, 0, x
	}

	r, g, b = r+m, g+m, b+m
	return r, g, b
}

// rgbToHSV converts red, green, and blue floating point values on the range
// 0.0 to 1.0 to hue, saturation and brightness values on the range 0.0 to 1.0
func rgbToHSV(r, g, b float32) (h, s, v float32) {
	var (
		xmax = max(r, g, b)
		xmin = min(r, g, b)
		c    = xmax - xmin
	)
	v = xmax
	switch {
	case c == 0:
		h = 0
	case v == r:
		h = (g - b) / (c * 6)
	case v == g:
		h = 1.0/3 + (b-r)/(c*6)
	case v == b:
		h = 2.0/3 + (r-g)/(c*6)
	}
	if h < 0 {
		h += 1
	}
	if xmax > 0 {
		s = c / xmax
	}
	return
}
