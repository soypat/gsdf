package glbuild

import (
	"encoding/json"
	"fmt"
	"io"
)

// IRMFHeader represents the JSON header for an Infinite Resolution Materials Format (IRMF) file.
type IRMFHeader struct {
	Author      string         `json:"author,omitempty"`
	License     string         `json:"license,omitempty"`
	Date        string         `json:"date,omitempty"`
	Encoding    string         `json:"encoding,omitempty"`
	IRMFVersion string         `json:"irmf"`
	GLSLVersion string         `json:"glslVersion,omitempty"`
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

// WriteIRMF creates the IRMF shader program for calculating SDF and writes it to the writer.
func (p *Programmer) WriteIRMF(w io.Writer, obj Shader3D, header IRMFHeader) (n int, objs []ShaderObject, err error) {
	// 1. Serialize and write the JSON header wrapped in /*...*/
	headerData, err := json.MarshalIndent(header, "", "  ")
	if err != nil {
		return 0, nil, fmt.Errorf("marshal error: %w", err)
	}

	ngot, err := fmt.Fprintf(w, "/*%s*/\n", headerData)
	n += ngot
	if err != nil {
		return n, nil, fmt.Errorf("fprintf error: %w", err)
	}

	// 2. Write the SDF function declarations.
	baseName, ngot, objs, err := p.WriteSDFDecl(w, obj)
	n += ngot
	if err != nil {
		return n, objs, fmt.Errorf("write SDF decl error: %w", err)
	}

	// 3. Append the IRMF-specific main function. (We assume 1-4 materials for now.)
	switch header.Language {
	case "glsl":
		ngot, err = fmt.Fprintf(w, `
void mainModel4(out vec4 materials, in vec3 xyz) {
  float d = %v(xyz);
  materials = vec4(d <= 0.0 ? 1.0 : 0.0, 0.0, 0.0, 0.0);
}
`,
			baseName)
		n += ngot
	case "wgsl":
		ngot, err = fmt.Fprintf(w, `
fn mainModel4(xyz: vec3f) -> vec4f {
  let d = %v(xyz);
  let materials = vec4f(d <= 0.0 ? 1.0 : 0.0, 0.0, 0.0, 0.0);
  return materials;
}
`,
			baseName)
		n += ngot
	default:
		return n, nil, fmt.Errorf("unknown IRMF language: %q", header.Language)
	}

	return n, objs, err
}
