package main

import (
	"fmt"
	"log"
	"runtime"
	"unicode/utf8"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/forge/textsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gsdfaux"
)

func init() {
	runtime.LockOSThread()
}

// scene generates the 3D object for rendering.
func scene(bld *gsdf.Builder) (glbuild.Shader3D, error) {
	var f textsdf.Font
	err := f.LoadTTFBytes(textsdf.ISO3098TTF())
	if err != nil {
		return nil, err
	}
	const text = "Hello world!"
	line, err := f.TextLine(text)
	if err != nil {
		return nil, err
	}
	// Find characteristic size of characters(glyphs/letters).
	len := utf8.RuneCountInString(text)
	sz := line.Bounds().Size()
	charWidth := sz.X / float32(len) // We then extrude based on the letter width.

	line = bld.Translate2D(line, -sz.X/2, 0) // Center text.
	return bld.Extrude(line, charWidth/3), bld.Err()
}

func main() {
	var bld gsdf.Builder
	shape, err := scene(&bld)
	shape = bld.Scale(shape, 0.3)
	if err != nil {
		log.Fatal("creating scene:", err)
	}
	fmt.Println("Running UI... compiling text shaders may take a while...")
	err = gsdfaux.UI(shape, gsdfaux.UIConfig{
		Width:  800,
		Height: 600,
	})
	if err != nil {
		log.Fatal("UI:", err)
	}
}
