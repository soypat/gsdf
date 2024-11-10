//go:build tinygo || !cgo

package gsdfaux

import (
	"errors"

	"github.com/soypat/gsdf/glbuild"
)

func ui(s glbuild.Shader3D, cfg UIConfig) error {
	return errors.New("require cgo for UI rendering")
}
