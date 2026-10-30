package textsdf

import (
	"testing"

	_ "embed"

	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

func TestABC(t *testing.T) {
	const badchar = "Abp8" //"ABbDdgoOpPqQR" // old badchar, takes too much time on CI.
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
	err = gsdfaux.RenderPNGFile("shape.png", sdfcpu, 128, nil)
	if err != nil {
		t.Fatal(err)
	}
}
