package gleval

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
)

// NewCPUSDF3 checks if the shader implements CPU evaluation and returns a [*SDF3CPU]
// ready for evaluation. It runs a single test evaluation on construction to catch
// missing evaluator implementations early.
//
// Pass nil as userData to Evaluate — the returned [*SDF3CPU] manages its own [VecPool].
func NewCPUSDF3(root bounder3) (*SDF3CPU, error) {
	sdf, err := AssertSDF3(root)
	if err != nil {
		return nil, fmt.Errorf("top level SDF cannot be CPU evaluated: %s", err.Error())
	}
	sdfcpu := SDF3CPU{
		SDF: sdf,
	}
	// Do a test evaluation with 1 value.
	bb := sdfcpu.Bounds()
	err = sdfcpu.Evaluate([]ms3.Vec{bb.Min}, []float32{0}, nil)
	if err != nil {
		return nil, err
	}

	return &sdfcpu, nil
}

// NewCPUSDF2 checks if the shader implements CPU evaluation and returns a [*SDF2CPU]
// ready for evaluation. It runs a single test evaluation on construction to catch
// missing evaluator implementations early.
//
// Pass nil as userData to Evaluate — the returned [*SDF2CPU] manages its own [VecPool].
func NewCPUSDF2(root bounder2) (*SDF2CPU, error) {
	sdf, err := AssertSDF2(root)
	if err != nil {
		return nil, fmt.Errorf("top level SDF cannot be CPU evaluated: %s", err.Error())
	}
	sdfcpu := SDF2CPU{
		SDF: sdf,
	}
	// Do a test evaluation with 1 value.
	bb := sdfcpu.Bounds()
	err = sdfcpu.Evaluate([]ms2.Vec{bb.Min}, []float32{0}, nil)
	if err != nil {
		return nil, err
	}

	return &sdfcpu, nil
}

// AssertSDF3 returns the argument as a [SDF3], or an error naming the concrete type
// if it does not implement CPU evaluation.
func AssertSDF3(s bounder3) (SDF3, error) {
	evaluator, ok := s.(SDF3)
	if !ok {
		return nil, fmt.Errorf("%T does not implement 3D evaluator", s)
	}
	return evaluator, nil
}

// AssertSDF2 returns the argument as a [SDF2], or an error naming the concrete type
// if it does not implement CPU evaluation.
func AssertSDF2(s bounder2) (SDF2, error) {
	evaluator, ok := s.(SDF2)
	if !ok {
		return nil, fmt.Errorf("%T does not implement 2D evaluator", s)
	}
	return evaluator, nil
}

// SDF3CPU wraps a [SDF3] with an owned [VecPool], providing a self-contained
// CPU evaluator. Construct with [NewCPUSDF3].
type SDF3CPU struct {
	SDF   SDF3
	vp    VecPool
	evals uint64
}

// Evaluate computes the SDF distance for each position in pos, writing results into dist.
// pos and dist must have equal non-zero length.
//
// Pass nil as userData to use the internal [VecPool]; in that case buffer leaks are
// detected automatically and reported as errors. Pass an external [VecPool] (or a type
// implementing VecPool() *VecPool) when composing evaluators in a larger tree.
func (sdf *SDF3CPU) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	useOwnVecPool := userData == nil
	if useOwnVecPool {
		userData = &sdf.vp
	} else if len(pos) != len(dist) {
		return errMismatchBufferLength
	} else if len(dist) == 0 {
		return errEmptyBuffers
	}
	err := sdf.SDF.Evaluate(pos, dist, userData)
	var err2 error
	if useOwnVecPool {
		// If sdf uses own vec pool then we also make sure all resources released on end.
		err2 = sdf.vp.AssertAllReleased()
	}
	if err != nil {
		if err2 != nil {
			return fmt.Errorf("VecPool leak: %s\nSDF3 error: %s", err2, err)
		}
		return err
	}
	if err2 != nil {
		return err2
	}
	sdf.evals += uint64(len(pos))
	return nil
}

// Bounds returns the bounding box of the underlying SDF.
func (sdf *SDF3CPU) Bounds() ms3.Box {
	return sdf.SDF.Bounds()
}

// Evaluations returns the total number of positions successfully evaluated over the lifetime of sdf.
func (sdf *SDF3CPU) Evaluations() uint64 { return sdf.evals }

// VecPool returns the internal pool, allowing callers to pass it as userData when
// composing this evaluator inside a larger SDF tree.
func (sdf *SDF3CPU) VecPool() *VecPool { return &sdf.vp }

// SDF2CPU wraps a [SDF2] with an owned [VecPool], providing a self-contained
// CPU evaluator. Construct with [NewCPUSDF2].
type SDF2CPU struct {
	SDF   SDF2
	vp    VecPool
	evals uint64
}

// Evaluate computes the SDF distance for each position in pos, writing results into dist.
// pos and dist must have equal non-zero length.
//
// Pass nil as userData to use the internal [VecPool]; in that case buffer leaks are
// detected automatically and reported as errors. Pass an external [VecPool] (or a type
// implementing VecPool() *VecPool) when composing evaluators in a larger tree.
func (sdf *SDF2CPU) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	useOwnVecPool := userData == nil
	if useOwnVecPool {
		userData = &sdf.vp
	} else if len(pos) != len(dist) {
		return errMismatchBufferLength
	} else if len(pos) == 0 {
		return errEmptyBuffers
	}
	err := sdf.SDF.Evaluate(pos, dist, userData)
	var err2 error
	if useOwnVecPool {
		err2 = sdf.vp.AssertAllReleased()
	}
	if err != nil {
		if err2 != nil {
			return fmt.Errorf("VecPool leak: %s\nSDF2 error: %s", err2, err)
		}
		return err
	}
	if err2 != nil {
		return err2
	}
	sdf.evals += uint64(len(pos))
	return nil
}

// Bounds returns the bounds of the underlying SDF.
func (sdf *SDF2CPU) Bounds() ms2.Box {
	return sdf.SDF.Bounds()
}

// Evaluations returns the total number of positions successfully evaluated over the lifetime of sdf.
func (sdf *SDF2CPU) Evaluations() uint64 { return sdf.evals }

// VecPool returns the internal pool, allowing callers to pass it as userData when
// composing this evaluator inside a larger SDF tree.
func (sdf *SDF2CPU) VecPool() *VecPool { return &sdf.vp }

// GetVecPool extracts a [*VecPool] from userData. It accepts either a [*VecPool] directly
// or any type that implements a VecPool() *VecPool method (as [SDF3CPU] and [SDF2CPU] do).
// Returns an error if userData is neither, or if the VecPool method returns nil.
func GetVecPool(userData any) (*VecPool, error) {
	vp, ok := userData.(*VecPool)
	if !ok {
		vper, ok := userData.(interface{ VecPool() *VecPool })
		if !ok {
			return nil, fmt.Errorf("want userData type glbuild.VecPool for CPU evaluations, got %T", userData)
		}
		vp = vper.VecPool()
		if vp == nil {
			return nil, fmt.Errorf("nil return value from VecPool method of %T", userData)
		}
	}
	return vp, nil
}

// VecPool is a set of typed buffer pools used by CPU SDF evaluators to reuse
// temporary slices across recursive calls, avoiding per-evaluation allocations.
//
// Each field is an independent pool for its element type. Evaluators call Acquire
// at the start of a function and Release (typically via defer) at the end, so the
// same backing memory is reused through the whole evaluation tree.
type VecPool struct {
	V3    bufPool[ms3.Vec]
	V2    bufPool[ms2.Vec]
	Float bufPool[float32]
}

// AssertAllReleased returns an error if any buffer is still acquired or if a
// Release error occurred since the last check. Called automatically by [SDF3CPU]
// and [SDF2CPU] after each evaluation when using the internal pool.
func (vp *VecPool) AssertAllReleased() error {
	err := vp.Float.assertAllReleased()
	if err != nil {
		return err
	}
	err = vp.V2.assertAllReleased()
	if err != nil {
		return err
	}
	err = vp.V3.assertAllReleased()
	if err != nil {
		return err
	}
	return nil
}

// SetMinAllocationLen sets the minimum length allocated when creating a new buffer for all buffer pools.
func (vp *VecPool) SetMinAllocationLen(minumumAlloca int) {
	if minumumAlloca < 0 {
		panic("invalid minimum allocation size")
	}
	vp.Float.minAllocation = minumumAlloca
	vp.V2.minAllocation = minumumAlloca
	vp.V3.minAllocation = minumumAlloca
}

// TotalSize returns the number of bytes allocated by all underlying buffers.
func (vp *VecPool) TotalSize() uint64 {
	return vp.Float.TotalSize() + vp.V2.TotalSize() + vp.V3.TotalSize()
}

// Deallocate frees all backing memory in all three pools. Panics if any buffer is
// still acquired. After this call the pool is valid for reuse.
func (vp *VecPool) Deallocate() {
	vp.Float.Deallocate()
	vp.V2.Deallocate()
	vp.V3.Deallocate()
}

// bufPool is a fixed-element-type pool backed by two parallel slices: _ins holds the
// allocated buffers and _acquired tracks which are in use. The parallel-slice layout
// avoids a struct-per-buffer allocation and keeps the hot path (Acquire search) cache-friendly.
type bufPool[T any] struct {
	_ins      [][]T
	_acquired []bool
	// releaseErr captures the error from a Release call. Release is typically called via
	// defer, so callers cannot inspect the return value; the error is surfaced later by
	// assertAllReleased. Cleared each time assertAllReleased is called.
	releaseErr error
	// minAllocation is the floor for new buffer lengths, reducing future re-allocations
	// when the caller knows the typical batch size in advance.
	minAllocation int
}

// Acquire returns a buffer of exactly length elements, reusing an existing free buffer
// if one is large enough or allocating a new one otherwise. The returned slice is valid
// until the matching [bufPool.Release] call. Panics if length <= 0.
func (bp *bufPool[T]) Acquire(length int) []T {
	if length <= 0 {
		panic("bufPool.Acquire: non-positive length")
	}
	for i, locked := range bp._acquired {
		if !locked && len(bp._ins[i]) >= length {
			bp._acquired[i] = true
			return bp._ins[i][:length]
		}
	}
	allocLen := length
	if bp.minAllocation > allocLen {
		allocLen = bp.minAllocation
	}
	newSlice := make([]T, allocLen)
	bp._ins = append(bp._ins, newSlice)
	bp._acquired = append(bp._acquired, true)
	return newSlice[:length]
}

var (
	errBufpoolReleaseUnaqcuired  = errors.New("release of unacquired resource")
	errBufpoolReleaseNonexistent = errors.New("release of nonexistent resource")
)

// Release returns buf to the pool. buf must be a slice previously returned by
// [bufPool.Acquire] — passing a sub-slice or an unrelated buffer is an error.
// The error is also stored in releaseErr for deferred callers that cannot inspect
// the return value.
func (bp *bufPool[T]) Release(buf []T) error {
	for i, instance := range bp._ins {
		if &instance[0] == &buf[0] {
			if !bp._acquired[i] {
				bp.releaseErr = errBufpoolReleaseUnaqcuired
				return bp.releaseErr
			}
			bp._acquired[i] = false
			return nil
		}
	}
	bp.releaseErr = errBufpoolReleaseNonexistent
	return bp.releaseErr
}

func (bp *bufPool[T]) assertAllReleased() error {
	for _, locked := range bp._acquired {
		if locked {
			return fmt.Errorf("locked %T resource found in glbuild.bufPool.assertAllReleased, memory leak?", *new(T))
		}
	}
	err := bp.releaseErr
	bp.releaseErr = nil // clear so the error is reported exactly once per occurrence
	return err
}

// TotalSize returns total amount of memory used by buffer in bytes.
func (bp *bufPool[T]) TotalSize() uint64 {
	var t T
	size := uint64(reflect.TypeOf(t).Size())
	var n uint64
	for i := range bp._ins {
		n += uint64(len(bp._ins[i]))
	}
	return size * n
}

// NumBuffers returns quantity of buffers allocated by the pool.
func (bp *bufPool[T]) NumBuffers() int {
	return len(bp._ins)
}

// NumFree returns the number of allocated buffers that are not currently acquired.
// In-use count = [bufPool.NumBuffers]() - NumFree().
func (bp *bufPool[T]) NumFree() (free int) {
	for _, b := range bp._acquired {
		if !b {
			free++
		}
	}
	return free
}

func (bp *bufPool[T]) String() string {
	alloc := bp.TotalSize()
	const (
		_ = 1 << (10 * iota)
		kB
		MB
	)
	units := "b"
	switch {
	case alloc > MB:
		alloc /= MB
		units = "MB"
	case alloc > kB:
		alloc /= kB
		units = "kB"
	}
	return fmt.Sprintf("bufPool{free:%d/%d  mem:%d%s}", bp.NumFree(), bp.NumBuffers(), alloc, units)
}

// Deallocate releases all backing memory and resets the pool to empty. Panics if any
// buffer is still acquired. After this call the pool is valid for reuse.
func (bp *bufPool[T]) Deallocate() {
	for _, locked := range bp._acquired {
		if locked {
			panic("bufPool.Deallocate called with active acquisitions")
		}
	}
	for i := range bp._ins {
		bp._ins[i] = nil
	}
	bp._ins = bp._ins[:0]
	bp._acquired = bp._acquired[:0]
	bp.releaseErr = nil
}
