package glbuild_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
)

var bld gsdf.Builder

func TestShaderNameDeduplication(t *testing.T) {
	// s1 and s2 are identical in name and body but different primitives.
	s1 := bld.NewCylinder(1, 2, 0)
	s2 := bld.NewCylinder(1, 2, 0)
	s1s1 := bld.Union(s1, s1)
	s1s2 := bld.Union(s1, s2)
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

func TestSprintShader(t *testing.T) {
	s1 := bld.NewCircle(1.0)
	s2 := bld.NewRectangle(2, 3)
	composite := bld.Difference2D(s1, s2)
	str := glbuild.SprintShader(composite)
	if str != "diff2D(circle2D,rect2D)" {
		t.Error(str)
	}
}
