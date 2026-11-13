// Package gpusdf implements GPU evaluation of signed distance fields using WebGPU.
//
// Native builds use wgpu-native via cgo, which runs on Metal (macOS), Vulkan, D3D12 and OpenGL ES.
// Builds targeting the browser (GOOS=js GOARCH=wasm) use the browser's WebGPU implementation.
// Builds without cgo (and TinyGo builds) compile but return an error on configuration.
//
// Types in this package follow a configure-then-use pattern: the zero value is ready to be configured
// and a call to Configure releases all previous resources and replaces them. GPU memory is allocated
// up front during Configure so evaluation does not allocate Go memory.
// Types are not safe for concurrent use and must not be copied after a call to Configure.
package gpusdf

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf/gpusdf/glbuild"
)

var (
	errNoGPU              = errors.New("gpusdf: GPU evaluation requires cgo (native) or GOOS=js and is not supported on TinyGo")
	errNotConfigured      = errors.New("gpusdf: not configured")
	errDeviceNotReady     = errors.New("gpusdf: device not configured")
	errBusy               = errors.New("gpusdf: evaluation in flight, call Read before submitting again")
	errNotSubmitted       = errors.New("gpusdf: no evaluation submitted")
	errMismatchLength     = errors.New("gpusdf: position and distance buffer length mismatch")
	errEmptyBuffers       = errors.New("gpusdf: empty buffers")
	errBatchExceeded      = errors.New("gpusdf: number of positions exceeds configured MaxBatch")
	errZeroWorkgroupSize  = errors.New("gpusdf: zero or negative workgroup size")
	errZeroMaxBatch       = errors.New("gpusdf: negative MaxBatch")
	errEmptySource        = errors.New("gpusdf: empty shader source")
	errWorkgroupSizeLimit = errors.New("gpusdf: workgroup size exceeds device limit")
)

// Flag return values for a [BackoffStrategy].
const (
	BackoffFlagGosched = time.Duration(-1)
	BackoffFlagNop     = time.Duration(-2)
)

// BackoffStrategy is the abstraction of a backoff strategy for waiting on GPU work to finish.
// It returns the amount of time to sleep for or a flag value:
//   - Returns [BackoffFlagNop]: Signal no yielding function should be called.
//     Useful for when the BackoffStrategy implements its own yield.
//   - Returns [BackoffFlagGosched]: Signal [runtime.Gosched] should be called.
//
// consecutiveBackoffs starts at 0 and increments by 1 every time the GPU is polled and work is not yet done.
type BackoffStrategy func(consecutiveBackoffs uint) (sleepOrFlag time.Duration)

// Do applies the backoff strategy by calling backoff(consecutiveBackoffs)
// and then the corresponding yield function for the returned value. See [BackoffStrategy].
func (backoff BackoffStrategy) Do(consecutiveBackoffs uint) {
	sleep := backoff(consecutiveBackoffs)
	switch sleep {
	case BackoffFlagNop:
		// No yield. Yield implemented by backoff.
	case BackoffFlagGosched:
		runtime.Gosched()
	default:
		time.Sleep(sleep)
	}
}

// PowerPreference is a hint for which GPU adapter to select on systems with more than one.
type PowerPreference uint8

const (
	PowerPreferenceUndefined PowerPreference = iota
	PowerPreferenceLowPower
	PowerPreferenceHighPerformance
)

// DeviceConfig configures a [Device].
type DeviceConfig struct {
	// ForceFallbackAdapter selects a software adapter (i.e: lavapipe, WARP) if available. Useful for CI.
	ForceFallbackAdapter bool
	// PowerPreference is a hint for adapter selection.
	PowerPreference PowerPreference
	// Backoff is used while waiting for GPU work to finish in blocking calls such as Evaluate.
	// If nil the calling goroutine blocks inside the GPU driver until the work is done.
	Backoff BackoffStrategy
}

// Device is a handle to a GPU device and its command queue. The zero value is ready to be configured with [Device.Configure].
type Device struct {
	state deviceState
	cfg   DeviceConfig
}

// Configure releases any GPU resources held by the device and requests a new adapter and device according to cfg.
// The device is requested with the maximum limits supported by the adapter.
func (d *Device) Configure(cfg DeviceConfig) error {
	d.Release()
	d.cfg = cfg
	return d.state.configure(cfg)
}

// Release releases GPU resources held by the device. Evaluators configured with the device must not be used after Release.
func (d *Device) Release() {
	d.state.release()
	d.cfg = DeviceConfig{}
}

// MaxWorkgroupSize returns the maximum compute workgroup size in the X dimension supported by the device.
// Returns 0 if device is not configured.
func (d *Device) MaxWorkgroupSize() int { return d.state.maxWorkgroupSize() }

// MaxStorageBuffers returns the maximum number of storage buffers per shader stage.
// Returns 0 if device is not configured.
func (d *Device) MaxStorageBuffers() int { return d.state.maxStorageBuffers() }

// AdapterInfo returns a human readable description of the GPU adapter in use.
func (d *Device) AdapterInfo() string { return d.state.adapterInfo() }

// ComputeConfig configures a GPU SDF evaluator.
type ComputeConfig struct {
	// Source is the WGSL source code of the compute program, usually generated by
	// [glbuild.Programmer.WriteComputeSDF3] or [glbuild.Programmer.WriteComputeSDF2].
	Source []byte
	// WorkgroupSize is the size of the compute workgroup in the X dimension and must match the size declared in Source:
	//  @compute @workgroup_size(<WorkgroupSize>, 1, 1)
	WorkgroupSize int
	// MaxBatch is the maximum number of positions evaluated in a single GPU dispatch. GPU buffers are sized according to MaxBatch
	// during configuration. Evaluate calls with more positions are split into several dispatches. If zero a default value is used.
	MaxBatch int
	// ShaderObjects contains buffer data and definitions required by shader for correct evaluation.
	// They are returned by the [glbuild.Programmer] shader generation methods. Data is uploaded to the GPU during configuration.
	ShaderObjects []glbuild.ShaderObject
}

const defaultMaxBatch = 1 << 16

func (cfg *ComputeConfig) validate(dev *Device) error {
	if dev == nil {
		return errDeviceNotReady
	} else if len(cfg.Source) == 0 {
		return errEmptySource
	} else if cfg.WorkgroupSize < 1 {
		return errZeroWorkgroupSize
	} else if cfg.MaxBatch < 0 {
		return errZeroMaxBatch
	}
	maxWG := dev.MaxWorkgroupSize()
	if maxWG == 0 {
		return errDeviceNotReady
	} else if cfg.WorkgroupSize > maxWG {
		return fmt.Errorf("%w: %d > %d", errWorkgroupSizeLimit, cfg.WorkgroupSize, maxWG)
	}
	return nil
}

func (cfg *ComputeConfig) maxBatch() int {
	if cfg.MaxBatch == 0 {
		return defaultMaxBatch
	}
	return cfg.MaxBatch
}

// SDF3 is a 3D signed distance field evaluated on the GPU. It implements the gleval.SDF3 interface.
type SDF3 struct {
	k     kernel
	bb    ms3.Box
	evals uint64
}

// Configure compiles the compute program in cfg.Source and allocates GPU memory for evaluation. bb is the
// bounding box of the SDF. Any previous GPU resources held by s are released.
func (s *SDF3) Configure(dev *Device, bb ms3.Box, cfg ComputeConfig) error {
	s.Release()
	if err := cfg.validate(dev); err != nil {
		return err
	}
	err := s.k.configure(dev, kernelConfig{
		source:      cfg.Source,
		wg:          cfg.WorkgroupSize,
		maxBatch:    cfg.maxBatch(),
		inputStride: [2]int{int(unsafe.Sizeof(ms3.Vec{}))},
		objs:        cfg.ShaderObjects,
	})
	if err != nil {
		return err
	}
	s.bb = bb
	return nil
}

// Release releases all GPU resources held by s.
func (s *SDF3) Release() {
	s.k.release()
	*s = SDF3{}
}

// Bounds returns the bounding box of the SDF. Implements gleval.SDF3.
func (s *SDF3) Bounds() ms3.Box { return s.bb }

// Evaluations returns total evaluations performed succesfully during sdf's lifetime.
func (s *SDF3) Evaluations() uint64 { return s.evals }

// Evaluate evaluates the SDF at positions pos and stores the result in dist. Blocks until the GPU is done.
// Implements gleval.SDF3.
func (s *SDF3) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	if len(pos) != len(dist) {
		return errMismatchLength
	} else if len(pos) == 0 {
		return errEmptyBuffers
	}
	maxBatch := s.k.maxBatch
	for len(pos) > 0 {
		n := min(len(pos), maxBatch)
		err := s.Submit(pos[:n])
		if err != nil {
			return err
		}
		err = s.Read(dist[:n])
		if err != nil {
			return err
		}
		pos = pos[n:]
		dist = dist[n:]
	}
	return nil
}

// Submit uploads positions to the GPU and submits evaluation of the SDF. It does not wait for the evaluation to finish.
// Results are retrieved with [SDF3.Read]. Use [SDF3.Poll] to check if results are ready without blocking.
// On WebAssembly targets Submit may block until the work is done.
func (s *SDF3) Submit(pos []ms3.Vec) error {
	return s.k.submit(sliceBytes(pos), nil, len(pos))
}

// Poll checks if the submitted evaluation has finished without blocking. Returns done=true if results can be read
// without blocking with [SDF3.Read].
func (s *SDF3) Poll() (done bool, err error) { return s.k.poll() }

// Read waits for the submitted evaluation to finish and stores the resulting distances in dist.
// len(dist) must be equal to the number of positions submitted. Waiting uses the [DeviceConfig] Backoff strategy.
func (s *SDF3) Read(dist []float32) error {
	err := s.k.read(sliceBytes(dist))
	if err == nil {
		s.evals += uint64(len(dist))
	}
	return err
}

// SDF2 is a 2D signed distance field evaluated on the GPU. It implements the gleval.SDF2 interface.
type SDF2 struct {
	k     kernel
	bb    ms2.Box
	evals uint64
}

// Configure compiles the compute program in cfg.Source and allocates GPU memory for evaluation. bb is the
// bounding box of the SDF. Any previous GPU resources held by s are released.
func (s *SDF2) Configure(dev *Device, bb ms2.Box, cfg ComputeConfig) error {
	s.Release()
	if err := cfg.validate(dev); err != nil {
		return err
	}
	err := s.k.configure(dev, kernelConfig{
		source:      cfg.Source,
		wg:          cfg.WorkgroupSize,
		maxBatch:    cfg.maxBatch(),
		inputStride: [2]int{int(unsafe.Sizeof(ms2.Vec{}))},
		objs:        cfg.ShaderObjects,
	})
	if err != nil {
		return err
	}
	s.bb = bb
	return nil
}

// Release releases all GPU resources held by s.
func (s *SDF2) Release() {
	s.k.release()
	*s = SDF2{}
}

// Bounds returns the bounding box of the SDF. Implements gleval.SDF2.
func (s *SDF2) Bounds() ms2.Box { return s.bb }

// Evaluations returns total evaluations performed succesfully during sdf's lifetime.
func (s *SDF2) Evaluations() uint64 { return s.evals }

// Evaluate evaluates the SDF at positions pos and stores the result in dist. Blocks until the GPU is done.
// Implements gleval.SDF2.
func (s *SDF2) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	if len(pos) != len(dist) {
		return errMismatchLength
	} else if len(pos) == 0 {
		return errEmptyBuffers
	}
	maxBatch := s.k.maxBatch
	for len(pos) > 0 {
		n := min(len(pos), maxBatch)
		err := s.Submit(pos[:n])
		if err != nil {
			return err
		}
		err = s.Read(dist[:n])
		if err != nil {
			return err
		}
		pos = pos[n:]
		dist = dist[n:]
	}
	return nil
}

// Submit uploads positions to the GPU and submits evaluation of the SDF. See [SDF3.Submit].
func (s *SDF2) Submit(pos []ms2.Vec) error {
	return s.k.submit(sliceBytes(pos), nil, len(pos))
}

// Poll checks if the submitted evaluation has finished without blocking. See [SDF3.Poll].
func (s *SDF2) Poll() (done bool, err error) { return s.k.poll() }

// Read waits for the submitted evaluation to finish and stores the resulting distances in dist. See [SDF3.Read].
func (s *SDF2) Read(dist []float32) error {
	err := s.k.read(sliceBytes(dist))
	if err == nil {
		s.evals += uint64(len(dist))
	}
	return err
}

// kernelConfig configures a compute kernel. The compute program bindings in group 0 are:
//   - inputs at bindings [0, numInputs)
//   - output f32 array at binding numInputs
//   - uniform vec4u parameters at binding numInputs+1. x component is set to number of elements to process.
//
// Shader objects are bound at group [glbuild.ObjectsBindGroup].
type kernelConfig struct {
	source   []byte
	wg       int
	maxBatch int
	// inputStride is the size of a single element of each input buffer. A zero stride means no input.
	inputStride [2]int
	objs        []glbuild.ShaderObject
}

func (kc *kernelConfig) numInputs() (n int) {
	for _, stride := range kc.inputStride {
		if stride > 0 {
			n++
		}
	}
	return n
}

type kernelState uint8

const (
	kernelIdle kernelState = iota
	kernelSubmitted
	kernelMapped
)

// sliceBytes returns the underlying memory of s as a byte slice without copying.
func sliceBytes[T any](s []T) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*int(unsafe.Sizeof(s[0])))
}

// poolSizes calculates the size in bytes of each pooled shader object binding.
func poolSizes(dst []int, objs []glbuild.ShaderObject) []int {
	dst = dst[:0]
	for i := range objs {
		obj := &objs[i]
		if !obj.IsBindable() || obj.Binding < 0 {
			continue
		}
		for len(dst) <= obj.Binding {
			dst = append(dst, 0)
		}
		end := obj.Offset*int(obj.Element.Size()) + obj.Size
		dst[obj.Binding] = max(dst[obj.Binding], end)
	}
	return dst
}

// copyPool copies the data of all shader objects with binding into dst.
func copyPool(dst []byte, binding int, objs []glbuild.ShaderObject) {
	for i := range objs {
		obj := &objs[i]
		if !obj.IsBindable() || obj.Binding != binding {
			continue
		}
		off := obj.Offset * int(obj.Element.Size())
		src := unsafe.Slice((*byte)(obj.Data), obj.Size)
		copy(dst[off:], src)
	}
}
