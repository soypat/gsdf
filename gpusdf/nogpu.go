//go:build tinygo || (!cgo && !js)

package gpusdf

import "github.com/soypat/gsdf/gpusdf/glbuild"

type deviceState struct{}

func (ds *deviceState) configure(cfg DeviceConfig) error { return errNoWGPU }
func (ds *deviceState) release()                         {}
func (ds *deviceState) maxWorkgroupSize() int            { return 0 }
func (ds *deviceState) maxStorageBuffers() int           { return 0 }
func (ds *deviceState) adapterInfo() string              { return "" }

type kernel struct {
	maxBatch int
}

func (k *kernel) configure(dev *Device, cfg kernelConfig) error { return errNoWGPU }
func (k *kernel) release()                                      {}
func (k *kernel) submit(in0, in1 []byte, n int) error           { return errNoWGPU }
func (k *kernel) poll() (bool, error)                           { return false, errNoWGPU }
func (k *kernel) read(dst []byte) error                         { return errNoWGPU }
func (k *kernel) setParam(y uint32)                             {}

func ui(s glbuild.Shader3D, cfg UIConfig) error { return errNoWGPU }
