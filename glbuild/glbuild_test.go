package glbuild_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

func TestShaderNameDeduplication(t *testing.T) {
	// s1 and s2 are identical in name and body but different primitives.
	s1, _ := gsdf.NewCylinder(1, 2, 0)
	s2, _ := gsdf.NewCylinder(1, 2, 0)
	s1s1 := gsdf.Union(s1, s1)
	s1s2 := gsdf.Union(s1, s2)
	s1Name := string(s1.AppendShaderName(nil))
	s2Name := string(s2.AppendShaderName(nil))
	if s1Name != s2Name {
		t.Error("expected same name, got\n", s1Name, "\n", s2Name)
	}
	decl := "float " + s1Name + "(vec3 p)"
	for _, obj := range []glbuild.Shader3D{s1s1, s1s2} {
		programmer := glbuild.NewDefaultProgrammer()
		source := new(bytes.Buffer)
		n, ssbos, err := programmer.WriteShaderToyVisualizerSDF3(source, obj)
		if n != source.Len() {
			t.Fatal("written length mismatch", err)
		} else if len(ssbos) > 0 {
			t.Fatal("unexpected ssbo")
		}
		if err != nil {
			t.Error(err)
		}
		src := source.String()
		declCount := strings.Count(src, decl)
		if declCount != 1 {
			t.Errorf("\n%s\nVisualizer: want one declaration, got %d", src, declCount)
		}
		source.Reset()
		n, ssbos, err = programmer.WriteComputeSDF3(source, obj)
		if n != source.Len() {
			t.Fatal("written length mismatch", err)
		} else if len(ssbos) > 0 {
			t.Fatal("unexpected ssbo")
		}
		if err != nil {
			t.Error(err)
		}
		src = source.String()
		declCount = strings.Count(src, decl)
		if declCount != 1 {
			t.Errorf("\n%s\nCompute: want one declaration, got %d", src, declCount)
		}
	}
}
