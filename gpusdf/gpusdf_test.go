package gpusdf_test

import (
	"math"
	"testing"
	"time"

	"github.com/soypat/geometry/ms2"
	"github.com/soypat/geometry/ms3"
	"github.com/soypat/gsdf"
	"github.com/soypat/gsdf/glbuild"
	"github.com/soypat/gsdf/gleval"
	"github.com/soypat/gsdf/gpusdf"
)

// testDevice returns a configured device or skips the test if no GPU is available.
func testDevice(t *testing.T) *gpusdf.Device {
	t.Helper()
	dev := new(gpusdf.Device)
	err := dev.Configure(gpusdf.DeviceConfig{})
	if err != nil {
		t.Skip("no GPU available:", err)
	}
	t.Cleanup(dev.Release)
	return dev
}

func TestNoDeviceErrors(t *testing.T) {
	var dev gpusdf.Device // Not configured.
	var sdf gpusdf.SDF3
	err := sdf.Configure(&dev, ms3.Box{}, gpusdf.ComputeConfig{Source: []byte("fn main(){}"), WorkgroupSize: 1})
	if err == nil {
		t.Fatal("expected error configuring SDF on unconfigured device")
	}
	err = sdf.Evaluate(make([]ms3.Vec, 1), make([]float32, 1), nil)
	if err == nil {
		t.Fatal("expected error evaluating unconfigured SDF")
	}
}

func grid2(bb ms2.Box, n int) []ms2.Vec {
	return ms2.AppendGrid(nil, bb.ScaleCentered(ms2.Vec{X: 1.2, Y: 1.2}), n, n)
}

func cmpDist(t *testing.T, want, got []float32) {
	t.Helper()
	const tol = 1e-3
	for i := range want {
		if math.Abs(float64(want[i]-got[i])) > tol {
			t.Fatalf("mismatch at %d: want %f, got %f", i, want[i], got[i])
		}
	}
}

func TestSDF3AsyncAndBatching(t *testing.T) {
	dev := testDevice(t)
	var bld gsdf.Builder
	shape := bld.Union(bld.NewSphere(1), bld.Translate(bld.NewBox(1, 2, 3, 0.1), 1, 0, 0))
	var prog bytes2
	p := glbuild.NewDefaultProgrammer()
	p.SetComputeInvocations(64, 1, 1)
	_, objs, err := p.WriteComputeSDF3(&prog, shape)
	if err != nil {
		t.Fatal(err)
	}
	var sdf gpusdf.SDF3
	defer sdf.Release()
	// Small MaxBatch forces Evaluate to split work in several dispatches.
	err = sdf.Configure(dev, shape.Bounds(), gpusdf.ComputeConfig{Source: prog, WorkgroupSize: 64, MaxBatch: 1000, ShaderObjects: objs})
	if err != nil {
		t.Fatal(err)
	}
	pos := ms3.AppendGrid(nil, shape.Bounds(), 20, 20, 20)
	want := make([]float32, len(pos))
	got := make([]float32, len(pos))
	cpu, err := gleval.NewCPUSDF3(shape)
	if err != nil {
		t.Fatal(err)
	}
	err = cpu.Evaluate(pos, want, cpu.VecPool())
	if err != nil {
		t.Fatal(err)
	}
	err = sdf.Evaluate(pos, got, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmpDist(t, want, got)
	if sdf.Evaluations() != uint64(len(pos)) {
		t.Errorf("want %d evaluations, got %d", len(pos), sdf.Evaluations())
	}

	// Asynchronous API.
	err = sdf.Submit(pos[:1000])
	if err != nil {
		t.Fatal(err)
	}
	if err = sdf.Submit(pos[:1000]); err == nil {
		t.Error("expected busy error submitting twice")
	}
	if err = sdf.Submit(pos[:1001]); err == nil {
		t.Error("expected error exceeding MaxBatch")
	}
	start := time.Now()
	for {
		done, err := sdf.Poll()
		if err != nil {
			t.Fatal(err)
		} else if done {
			break
		} else if time.Since(start) > 10*time.Second {
			t.Fatal("timeout polling")
		}
		time.Sleep(time.Millisecond)
	}
	clear(got)
	err = sdf.Read(got[:1000])
	if err != nil {
		t.Fatal(err)
	}
	cmpDist(t, want[:1000], got[:1000])
}

func TestBuiltin2D(t *testing.T) {
	dev := testDevice(t)
	var bld gsdf.Builder
	vertices := []ms2.Vec{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 2}, {X: 0, Y: 1}}
	lines := [][2]ms2.Vec{{{X: 0, Y: 0}, {X: 1, Y: 1}}, {{X: 1, Y: 1}, {X: 2, Y: -1}}}
	const width = 0.2

	t.Run("polygon", func(t *testing.T) {
		cpu := bld.NewPolygon(vertices)
		gpu := gpusdf.Polygon{Vertices: vertices}
		defer gpu.Release()
		err := gpu.Configure(dev, gpusdf.KernelConfig{})
		if err != nil {
			t.Fatal(err)
		}
		cmp2(t, cpu, &gpu)
	})
	t.Run("lines", func(t *testing.T) {
		cpu := bld.NewLines2D(lines, width)
		gpu := gpusdf.Lines2D{Lines: lines, Width: width}
		defer gpu.Release()
		err := gpu.Configure(dev, gpusdf.KernelConfig{})
		if err != nil {
			t.Fatal(err)
		}
		cmp2(t, cpu, &gpu)
	})
	t.Run("displacemulti", func(t *testing.T) {
		elem := bld.NewCircle(0.3)
		cpu := bld.TranslateMulti2D(elem, vertices)
		gpu := gpusdf.DisplaceMulti2D{Displacements: vertices}
		defer gpu.Release()
		err := gpu.Configure(dev, glbuild.NewDefaultProgrammer(), elem, gpusdf.KernelConfig{WorkgroupSize: 32})
		if err != nil {
			t.Fatal(err)
		}
		cmp2(t, cpu, &gpu)
	})
	t.Run("batcher", func(t *testing.T) {
		var b gpusdf.Batcher
		defer b.Release()
		err := b.Configure(dev, gpusdf.KernelConfig{MaxBatch: 3})
		if err != nil {
			t.Fatal(err)
		}
		A := []float32{1, -2, 3, 4, 5}
		B := []float32{2, 3, -1, 4, 0}
		dst := make([]float32, len(A))
		err = b.ExecuteRawBinaryOperation([]byte("return min(a,b);"), dst, A, B)
		if err != nil {
			t.Fatal(err)
		}
		cmpDist(t, []float32{1, -2, -1, 4, 0}, dst)
		err = b.ExecuteRawBinaryOperation([]byte("return max(a,-b);"), dst, A, B)
		if err != nil {
			t.Fatal(err)
		}
		cmpDist(t, []float32{1, -2, 3, 4, 5}, dst)
	})
}

func cmp2(t *testing.T, cpu glbuild.Shader2D, gpu gleval.SDF2) {
	t.Helper()
	sdfcpu, err := gleval.NewCPUSDF2(cpu)
	if err != nil {
		t.Fatal(err)
	}
	pos := grid2(cpu.Bounds(), 64)
	want := make([]float32, len(pos))
	got := make([]float32, len(pos))
	err = sdfcpu.Evaluate(pos, want, sdfcpu.VecPool())
	if err != nil {
		t.Fatal(err)
	}
	err = gpu.Evaluate(pos, got, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmpDist(t, want, got)
}

// bytes2 is a minimal io.Writer that appends to a byte slice, usable directly as ComputeConfig.Source.
type bytes2 []byte

func (b *bytes2) Write(p []byte) (int, error) {
	*b = append(*b, p...)
	return len(p), nil
}
