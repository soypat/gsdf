package gsdfaux

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestColorGradient(t *testing.T) {
	const Xdim = 256
	img := image.NewRGBA(image.Rect(0, 0, Xdim, Xdim))
	conv := ColorConversionLinearGradient(Xdim, color.White, red)
	var x float32 = -Xdim / 2
	for i := range Xdim {
		for j := range Xdim {
			img.Set(i, j, conv(x))
		}
		x += 1
	}
	fp, _ := os.Create("test.png")
	png.Encode(fp, img)
}
