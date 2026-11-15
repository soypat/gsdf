package glbuild

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// IRMFHeaderV1 represents the JSON header for an Infinite Resolution Materials Format (IRMF) file.
// gsdf generates WGSL, so [IRMFHeaderV1.WriteIRMF] writes WGSL shader bodies and the
// header's Language field must be "wgsl".
type IRMFHeaderV1 struct {
	Author      string         `json:"author,omitempty"`
	License     string         `json:"license,omitempty"`
	Date        string         `json:"date,omitempty"`
	Encoding    string         `json:"encoding,omitempty"`
	IRMFVersion string         `json:"irmf"`
	Language    string         `json:"language"`
	Materials   []string       `json:"materials"`
	Max         [3]float32     `json:"max"`
	Min         [3]float32     `json:"min"`
	Notes       string         `json:"notes,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	Title       string         `json:"title,omitempty"`
	Units       string         `json:"units"`
	Version     string         `json:"version,omitempty"`
}

// WriteIRMF writes an IRMF document containing the WGSL shader program for obj to w.
// It returns the number of bytes written, the shader objects referenced by the
// program and any error encountered.
//
// The header's Language field is set to "wgsl" if empty; any other language is
// rejected because gsdf only generates WGSL. The IRMFVersion field is set to
// "1.0" if empty. IRMF added WGSL support as an amendment while keeping the
// spec version at 1.0.
func (ih IRMFHeaderV1) WriteIRMF(w io.Writer, obj Shader3D, programmer *Programmer) (n int, objs []ShaderObject, err error) {
	if programmer == nil {
		return 0, nil, errors.New("programmer is nil")
	}
	if ih.Language == "" {
		ih.Language = "wgsl"
	} else if ih.Language != "wgsl" {
		return 0, nil, fmt.Errorf("unsupported IRMF language %q: gsdf generates WGSL only", ih.Language)
	}
	if ih.IRMFVersion == "" {
		ih.IRMFVersion = "1.0"
	}

	// 1. Serialize and write the JSON header wrapped in /*...*/
	headerData, err := json.MarshalIndent(ih, "", "  ")
	if err != nil {
		return 0, nil, fmt.Errorf("marshaling IRMF header: %w", err)
	}
	ngot, err := fmt.Fprintf(w, "/*%s*/\n", headerData)
	n += ngot
	if err != nil {
		return n, nil, fmt.Errorf("writing IRMF header: %w", err)
	}

	// 2. Write the SDF function declarations.
	baseName, ngot, objs, err := programmer.WriteSDFDecl(w, obj)
	n += ngot
	if err != nil {
		return n, objs, fmt.Errorf("writing SDF declarations: %w", err)
	}

	// 3. Append the IRMF-specific entry point. (We assume 1-4 materials for now.)
	ngot, err = fmt.Fprintf(w, irmfMainModel4WGSL, baseName)
	n += ngot
	if err != nil {
		return n, objs, fmt.Errorf("writing IRMF entry point: %w", err)
	}
	return n, objs, nil
}

// irmfMainModel4WGSL is the IRMF entry point for models with 1-4 materials.
// See https://github.com/irmf/irmf/blob/master/spec.
const irmfMainModel4WGSL = `
fn mainModel4(xyz: vec3f) -> vec4f {
  let d = %s(xyz);
  let material = select(0.0, 1.0, d <= 0.0);
  return vec4f(material, 0.0, 0.0, 0.0);
}
`
