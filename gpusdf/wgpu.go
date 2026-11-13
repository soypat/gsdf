//go:build !tinygo && (cgo || js)

package gpusdf

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/oliverbestmann/webgpu/wgpu"
	"github.com/soypat/gsdf/gpusdf/glbuild"
)

type deviceState struct {
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue
	limits   wgpu.Limits
	backoff  BackoffStrategy
}

func (ds *deviceState) configure(cfg DeviceConfig) (err error) {
	defer func() {
		if err != nil {
			ds.release()
		}
	}()
	ds.instance = wgpu.CreateInstance(nil)
	if ds.instance == nil {
		return errors.New("gpusdf: WebGPU not available")
	}
	var pref wgpu.PowerPreference
	switch cfg.PowerPreference {
	case PowerPreferenceLowPower:
		pref = wgpu.PowerPreferenceLowPower
	case PowerPreferenceHighPerformance:
		pref = wgpu.PowerPreferenceHighPerformance
	}
	ds.adapter, err = ds.instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		ForceFallbackAdapter: cfg.ForceFallbackAdapter,
		PowerPreference:      pref,
	})
	if err != nil {
		return fmt.Errorf("gpusdf: requesting adapter: %w", err)
	}
	// Request maximum adapter limits relevant to SDF evaluation, default WebGPU limits are conservative (i.e: 8 storage buffers,
	// 256 invocations per workgroup). Other limits are left undefined (default) since requesting all adapter limits
	// fails on some implementations, i.e: browser adapters reporting limits they do not accept.
	adapterLimits := ds.adapter.GetLimits()
	limits := wgpu.DefaultLimits()
	limits.MaxStorageBuffersPerShaderStage = adapterLimits.MaxStorageBuffersPerShaderStage
	limits.MaxStorageBufferBindingSize = adapterLimits.MaxStorageBufferBindingSize
	limits.MaxBufferSize = adapterLimits.MaxBufferSize
	limits.MaxComputeInvocationsPerWorkgroup = adapterLimits.MaxComputeInvocationsPerWorkgroup
	limits.MaxComputeWorkgroupSizeX = adapterLimits.MaxComputeWorkgroupSizeX
	limits.MaxComputeWorkgroupsPerDimension = adapterLimits.MaxComputeWorkgroupsPerDimension
	ds.device, err = ds.adapter.RequestDevice(&wgpu.DeviceDescriptor{
		Label:          "gsdf",
		RequiredLimits: &limits,
	})
	if err != nil {
		return fmt.Errorf("gpusdf: requesting device: %w", err)
	}
	ds.queue = ds.device.GetQueue()
	ds.limits = ds.device.GetLimits()
	ds.backoff = cfg.Backoff
	return nil
}

func (ds *deviceState) release() {
	if ds.queue != nil {
		ds.queue.Release()
	}
	if ds.device != nil {
		ds.device.Release()
	}
	if ds.adapter != nil {
		ds.adapter.Release()
	}
	if ds.instance != nil {
		ds.instance.Release()
	}
	*ds = deviceState{}
}

func (ds *deviceState) maxWorkgroupSize() int {
	if ds.device == nil {
		return 0
	}
	return int(min(ds.limits.MaxComputeInvocationsPerWorkgroup, ds.limits.MaxComputeWorkgroupSizeX))
}

func (ds *deviceState) maxStorageBuffers() int {
	if ds.device == nil {
		return 0
	}
	return int(ds.limits.MaxStorageBuffersPerShaderStage)
}

func (ds *deviceState) adapterInfo() string {
	if ds.adapter == nil {
		return ""
	}
	info := ds.adapter.GetInfo()
	if info.Vendor == "" && info.Device == "" && info.Description == "" {
		return "WebGPU adapter" // Browsers may hide adapter information.
	}
	return fmt.Sprintf("%s %s (%s, %s)", info.Vendor, info.Device, info.BackendType.String(), info.AdapterType.String())
}

type kernel struct {
	dev       *Device
	pipeline  *wgpu.ComputePipeline
	bindGroup [2]*wgpu.BindGroup // bind groups 0 (I/O) and 1 (shader objects).
	inputs    [2]*wgpu.Buffer
	output    *wgpu.Buffer
	staging   *wgpu.Buffer
	paramsBuf *wgpu.Buffer
	pools     []*wgpu.Buffer
	params    [4]uint32
	strides   [2]int
	wg        int
	maxBatch  int
	n         int // number of elements in flight.
	state     kernelState
	mapStatus wgpu.MapAsyncStatus
	mapCb     wgpu.BufferMapCallback
}

func (k *kernel) configure(dev *Device, cfg kernelConfig) (err error) {
	k.release()
	defer func() {
		if err != nil {
			k.release()
		}
	}()
	ds := &dev.state
	if ds.device == nil {
		return errDeviceNotReady
	}
	numInputs := cfg.numInputs()
	if cfg.wg < 1 {
		return errZeroWorkgroupSize
	} else if numInputs == 0 {
		return errors.New("gpusdf: kernel requires input")
	}
	maxBindingSize := min(ds.limits.MaxStorageBufferBindingSize, ds.limits.MaxBufferSize)
	for _, stride := range cfg.inputStride {
		if uint64(stride)*uint64(cfg.maxBatch) > maxBindingSize {
			return fmt.Errorf("gpusdf: MaxBatch %d too large for device storage buffer limit %d", cfg.maxBatch, maxBindingSize)
		}
	}
	device := ds.device
	module, err := device.TryCreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label:      "gsdf compute",
		WGSLSource: &wgpu.ShaderSourceWGSL{Code: string(cfg.source)},
	})
	if err != nil {
		return fmt.Errorf("gpusdf: compiling shader: %w", err)
	}
	defer module.Release()
	k.pipeline, err = device.TryCreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "gsdf compute",
		Compute: wgpu.ProgrammableStageDescriptor{
			Module:     module,
			EntryPoint: glbuild.ComputeEntryPoint,
		},
	})
	if err != nil {
		return fmt.Errorf("gpusdf: creating compute pipeline: %w", err)
	}

	// Create I/O buffers.
	var entries [4]wgpu.BindGroupEntry
	nentries := 0
	for i, stride := range cfg.inputStride {
		if stride <= 0 {
			continue
		}
		k.inputs[i], err = device.TryCreateBuffer(&wgpu.BufferDescriptor{
			Label: "gsdf input",
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
			Size:  align4(uint64(stride) * uint64(cfg.maxBatch)),
		})
		if err != nil {
			return err
		}
		entries[nentries] = wgpu.BindGroupEntry{Binding: uint32(nentries), Buffer: k.inputs[i], Size: wgpu.WholeSize}
		nentries++
	}
	outSize := uint64(4 * cfg.maxBatch)
	k.output, err = device.TryCreateBuffer(&wgpu.BufferDescriptor{
		Label: "gsdf output",
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
		Size:  outSize,
	})
	if err != nil {
		return err
	}
	entries[nentries] = wgpu.BindGroupEntry{Binding: uint32(nentries), Buffer: k.output, Size: wgpu.WholeSize}
	nentries++
	k.staging, err = device.TryCreateBuffer(&wgpu.BufferDescriptor{
		Label: "gsdf staging",
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		Size:  outSize,
	})
	if err != nil {
		return err
	}
	k.paramsBuf, err = device.TryCreateBuffer(&wgpu.BufferDescriptor{
		Label: "gsdf params",
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		Size:  uint64(unsafe.Sizeof(k.params)),
	})
	if err != nil {
		return err
	}
	entries[nentries] = wgpu.BindGroupEntry{Binding: uint32(nentries), Buffer: k.paramsBuf, Size: wgpu.WholeSize}
	nentries++
	layout0 := k.pipeline.GetBindGroupLayout(0)
	defer layout0.Release()
	k.bindGroup[0], err = device.TryCreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "gsdf I/O",
		Layout:  layout0,
		Entries: entries[:nentries],
	})
	if err != nil {
		return fmt.Errorf("gpusdf: creating I/O bind group: %w", err)
	}

	// Upload shader objects. Objects of the same type are pooled into a single binding.
	k.bindGroup[1], k.pools, err = createPoolBindGroup(dev, k.pipeline, cfg.objs, k.pools[:0])
	if err != nil {
		return err
	}
	k.dev = dev
	k.strides = cfg.inputStride
	k.wg = cfg.wg
	k.maxBatch = cfg.maxBatch
	k.mapCb = k.onMap // Store method value once to avoid allocating on every submit.
	return nil
}

// createPoolBindGroup uploads shader object data to GPU buffers pooled by binding and creates the corresponding bind group.
// Returns nil bind group if there are no bindable objects.
func createPoolBindGroup(dev *Device, pipeline interface {
	GetBindGroupLayout(uint32) *wgpu.BindGroupLayout
}, objs []glbuild.ShaderObject, pools []*wgpu.Buffer) (*wgpu.BindGroup, []*wgpu.Buffer, error) {
	sizes := poolSizes(nil, objs)
	if len(sizes) == 0 {
		return nil, pools, nil
	}
	ds := &dev.state
	if limit := ds.maxStorageBuffers(); len(sizes)+2 > limit {
		return nil, pools, fmt.Errorf("gpusdf: shader requires %d storage buffers, device limit is %d", len(sizes)+2, limit)
	}
	var scratch []byte
	entries := make([]wgpu.BindGroupEntry, len(sizes))
	for binding, size := range sizes {
		if size == 0 {
			return nil, pools, fmt.Errorf("gpusdf: shader object binding %d is empty", binding)
		}
		scratch = append(scratch[:0], make([]byte, align4(uint64(size)))...)
		copyPool(scratch, binding, objs)
		buf, err := ds.device.TryCreateBufferInit(&wgpu.BufferInitDescriptor{
			Label:    "gsdf objects",
			Contents: scratch,
			Usage:    wgpu.BufferUsageStorage,
		})
		if err != nil {
			return nil, pools, err
		}
		pools = append(pools, buf)
		entries[binding] = wgpu.BindGroupEntry{Binding: uint32(binding), Buffer: buf, Size: wgpu.WholeSize}
	}
	layout := pipeline.GetBindGroupLayout(glbuild.ObjectsBindGroup)
	defer layout.Release()
	bg, err := ds.device.TryCreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "gsdf objects",
		Layout:  layout,
		Entries: entries,
	})
	if err != nil {
		return nil, pools, fmt.Errorf("gpusdf: creating shader object bind group: %w", err)
	}
	return bg, pools, nil
}

func (k *kernel) onMap(status wgpu.MapAsyncStatus) { k.mapStatus = status }

func (k *kernel) release() {
	if k.state == kernelMapped && k.staging != nil {
		k.staging.TryUnmap()
	}
	for _, b := range k.bindGroup {
		if b != nil {
			b.Release()
		}
	}
	for _, b := range k.inputs {
		if b != nil {
			b.Release()
		}
	}
	for _, b := range [...]*wgpu.Buffer{k.output, k.staging, k.paramsBuf} {
		if b != nil {
			b.Release()
		}
	}
	for i, b := range k.pools {
		b.Release()
		k.pools[i] = nil
	}
	if k.pipeline != nil {
		k.pipeline.Release()
	}
	pools := k.pools[:0]
	*k = kernel{pools: pools}
}

func (k *kernel) submit(in0, in1 []byte, n int) (err error) {
	if k.pipeline == nil {
		return errNotConfigured
	} else if k.state != kernelIdle {
		return errBusy
	} else if n == 0 {
		return errEmptyBuffers
	} else if n > k.maxBatch {
		return errBatchExceeded
	}
	ds := &k.dev.state
	queue := ds.queue
	for i, in := range [2][]byte{in0, in1} {
		if k.strides[i] == 0 {
			continue
		} else if len(in) != n*k.strides[i] {
			return errMismatchLength
		}
		err = queue.TryWriteBuffer(k.inputs[i], 0, in)
		if err != nil {
			return err
		}
	}
	k.params[0] = uint32(n)
	err = queue.TryWriteBuffer(k.paramsBuf, 0, sliceBytes(k.params[:]))
	if err != nil {
		return err
	}
	encoder, err := ds.device.TryCreateCommandEncoder(nil)
	if err != nil {
		return err
	}
	defer encoder.Release()
	pass := encoder.BeginComputePass(nil)
	pass.SetPipeline(k.pipeline)
	pass.SetBindGroup(0, k.bindGroup[0], nil)
	if k.bindGroup[1] != nil {
		pass.SetBindGroup(glbuild.ObjectsBindGroup, k.bindGroup[1], nil)
	}
	// Dispatch in 2D if number of workgroups exceeds limit per dimension. Shader handles index calculation.
	groups := (n + k.wg - 1) / k.wg
	maxPerDim := int(ds.limits.MaxComputeWorkgroupsPerDimension)
	gx := min(groups, maxPerDim)
	gy := (groups + gx - 1) / gx
	pass.DispatchWorkgroups(uint32(gx), uint32(gy), 1)
	err = pass.TryEnd()
	pass.Release() // Must release pass before submitting: https://github.com/gfx-rs/wgpu-native/issues/412
	if err != nil {
		return err
	}
	size := uint64(4 * n)
	err = encoder.TryCopyBufferToBuffer(k.output, 0, k.staging, 0, size)
	if err != nil {
		return err
	}
	cmd, err := encoder.TryFinish(nil)
	if err != nil {
		return err
	}
	queue.Submit(cmd)
	cmd.Release()
	k.mapStatus = 0
	k.n = n
	k.state = kernelSubmitted
	err = k.staging.TryMapAsync(wgpu.MapModeRead, 0, size, k.mapCb)
	if err != nil {
		k.state = kernelIdle
		return err
	}
	return nil
}

func (k *kernel) poll() (done bool, err error) {
	switch k.state {
	case kernelIdle:
		return false, errNotSubmitted
	case kernelMapped:
		return true, nil
	}
	if k.mapStatus == 0 {
		k.dev.state.device.Poll(false, nil)
	}
	return k.checkMapStatus()
}

func (k *kernel) checkMapStatus() (done bool, err error) {
	switch k.mapStatus {
	case 0:
		return false, nil
	case wgpu.MapAsyncStatusSuccess:
		k.state = kernelMapped
		return true, nil
	}
	k.state = kernelIdle
	return false, fmt.Errorf("gpusdf: mapping result buffer failed with status %s", k.mapStatus.String())
}

func (k *kernel) wait() error {
	if k.state == kernelIdle {
		return errNotSubmitted
	}
	ds := &k.dev.state
	backoff := ds.backoff
	for i := uint(0); ; i++ {
		if backoff == nil && k.mapStatus == 0 {
			ds.device.Poll(true, nil)
		}
		done, err := k.poll()
		if err != nil || done {
			return err
		}
		if backoff != nil {
			backoff.Do(i)
		} else if i > 1<<20 {
			return errors.New("gpusdf: timeout waiting for GPU")
		}
	}
}

func (k *kernel) read(dst []byte) error {
	if k.state == kernelIdle {
		return errNotSubmitted
	} else if len(dst) != 4*k.n {
		return errMismatchLength
	}
	err := k.wait()
	if err != nil {
		return err
	}
	mapped := k.staging.GetMappedRange(0, uint(len(dst)))
	copy(dst, mapped)
	k.state = kernelIdle
	return k.staging.TryUnmap()
}

func align4(n uint64) uint64 { return (n + 3) &^ 3 }

// setParam sets the y component of the uniform parameters passed to the compute program on the next submit.
func (k *kernel) setParam(y uint32) { k.params[1] = y }
