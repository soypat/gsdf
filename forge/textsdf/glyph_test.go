package textsdf

import (
	"testing"

	_ "embed"

	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gsdfaux"
)

//go:embed iso-3098.ttf
var _isonormTTF []byte

func TestABC(t *testing.T) {
	var f Font
	err := f.LoadTTFBytes(_isonormTTF)
	if err != nil {
		t.Fatal(err)
	}
	shape, err := f.TextLine("e")
	// shape, err := f.Glyph('A')
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
