package gsdfaux

import (
	"bytes"
	"strings"
	"testing"

	"github.com/soypat/gsdf"
)

// TestRenderIRMFWGSL checks that IRMF output contains a WGSL program and no
// GLSL syntax. See https://github.com/irmf/irmf/blob/master/spec.
func TestRenderIRMFWGSL(t *testing.T) {
	var bld gsdf.Builder
	s := bld.NewSphere(1)
	if err := bld.Err(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	name, err := renderIRMF(&buf, s)
	if err != nil {
		t.Fatal(err)
	} else if name != "IRMF program" {
		t.Errorf("got filename %q, want %q", name, "IRMF program")
	}

	out := buf.String()
	for _, want := range []string{
		`"irmf": "1.0"`,
		`"language": "wgsl"`,
		`"units": "mm"`,
		"fn mainModel4(xyz: vec3f) -> vec4f",
		"select(0.0, 1.0, d <= 0.0)", // WGSL has no ?: ternary operator.
	} {
		if !strings.Contains(out, want) {
			t.Errorf("IRMF output missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"vec4(", "void mainModel4", "#version", "?"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("IRMF output contains GLSL syntax %q:\n%s", unwanted, out)
		}
	}
}
