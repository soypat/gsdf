//go:build tinygo || !cgo

package gsdfaux

import "github.com/soypat/gsdf/glbuild"

func ui(s glbuild.Shader3D, cfg UIConfig) error {
	return errors.new("require cgo for UI rendering")
}
