package textsdf

import (
	"fmt"
	"testing"

	_ "embed"

	"github.com/golang/freetype/truetype"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
	"golang.org/x/image/math/fixed"
)

func TestABC(t *testing.T) {
	const okchar = "BCDEFGHIJK"
	const badchar = "iB~"
	var f Font
	err := f.LoadTTFBytes(ISO3098TTF())
	if err != nil {
		t.Fatal(err)
	}
	shape, err := f.TextLine(badchar)
	if err != nil {
		t.Fatal(err)
	}
	sdfcpu, err := gleval.NewCPUSDF2(shape)
	if err != nil {
		t.Fatal(err)
	}
	err = gsdfaux.RenderPNGFile("shape.png", sdfcpu, 512, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func Test(t *testing.T) {
	ttf, err := truetype.Parse(_iso3098TTF)
	if err != nil {
		panic(err)
	}
	scale := fixed.Int26_6(ttf.FUnitsPerEm())
	hm := ttf.HMetric(scale, 'E')
	fmt.Println(hm.AdvanceWidth, int(hm.AdvanceWidth))
	t.Error(hm)
	// var g truetype.GlyphBuf
	// err = g.Load(ttf, , 'B', font.HintingFull)
	// if err != nil {
	// 	panic(err)
	// }

}
