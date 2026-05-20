package gsdfaux

import (
	"fmt"
	"os"

	"github.com/soypat/gsdf/glbuild"
)

func renderIRMF(cfg RenderConfig, s glbuild.Shader3D, language string) error {
	irmfWatch := stopwatch()
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
	_, objects, err := header.WriteIRMF(cfg.IRMFOutput, s, programmer)
	if err != nil {
		return fmt.Errorf("writing IRMF: %w", err)
	}

	for i := range objects {
		if objects[i].IsBindable() {
			return fmt.Errorf("bindable objects unsupported for IRMF shader generation: %v", objects[i])
		}
	}

	filename := "model.irmf"
	if fp, ok := cfg.IRMFOutput.(*os.File); ok {
		filename = fp.Name()
	}
	fmt.Printf("[%v] wrote %v\n", irmfWatch(), filename)
	return nil
}
