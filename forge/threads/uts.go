package threads

import (
	"github.com/soypat/gsdf/glbuild"
)

// Unified thread standard.
// Example: UNC 1/4 with external threading would be
//
//	UTS{D:1.0/4.0, TPI:20, Ext: true}
type UTS struct {
	// Diameter.
	D float32
	// Threads per inch, equivalent to thread revolutions per 25.4mm.
	TPI float32
	// External or internal thread.
	Ext bool
}

var _ Threader = UTS{} // Compile time interface implementation guarantee.

func (uts UTS) ThreadParams() Parameters {
	p := basic{D: uts.D, P: 1.0 / uts.TPI}.ThreadParams()
	// TODO(soypat) add imperial hex flat-to-flat. See NPT for what that could look like.
	return p
}

func (uts UTS) Thread() (glbuild.Shader2D, error) {
	return ISO{D: uts.D, P: 1.0 / uts.TPI, Ext: uts.Ext}.Thread()
}
