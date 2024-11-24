package textsdf

import (
	"testing"

	_ "embed"

	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
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
