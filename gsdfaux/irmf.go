package gsdfaux

import (
	"fmt"
	"io"
	"os"

	"github.com/soypat/gsdf/glbuild"
)

// renderIRMF writes the IRMF program for s to w using the given shader language.
// It returns the name of w suitable for logging.
func renderIRMF(w io.Writer, s glbuild.Shader3D, language string) (string, error) {
	bb := s.Bounds()
	min, max := bb.Min, bb.Max
	header := glbuild.IRMFHeaderV1{
		IRMFVersion: "1.0",
		Language:    language,
		Materials:   []string{"material0"},
		Min:         [3]float32{min.X, min.Y, min.Z},
		Max:         [3]float32{max.X, max.Y, max.Z},
		Units:       "mm",
	}

	var objects []glbuild.ShaderObject
	programmer := glbuild.NewDefaultProgrammer()
	_, objects, err := header.WriteIRMF(w, s, programmer)
	if err != nil {
		return "", fmt.Errorf("writing IRMF: %w", err)
	}

	for i := range objects {
		if objects[i].IsBindable() {
			return "", fmt.Errorf("bindable objects unsupported for IRMF shader generation: %v", objects[i])
		}
	}

	filename := "IRMF program"
	if fp, ok := w.(*os.File); ok {
		filename = fp.Name()
	}
	return filename, nil
}
