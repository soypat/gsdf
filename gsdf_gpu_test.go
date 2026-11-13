//go:build !tinygo && cgo

package gsdf_test

import (
	"flag"
	"log"
	"os"
	"testing"

	"github.com/soypat/gsdf/gpusdf"
	"github.com/soypat/gsdf/gsdfaux"
)

var (
	flagUIOnFail    = flag.Bool("gsdf.uionfail", false, "Open UI window displaying the shape on GPU test failure.")
	flagFallbackGPU = flag.Bool("gsdf.fallbackgpu", false, "Use software GPU adapter (i.e. lavapipe). Useful for CI.")
	flagRequireGPU  = flag.Bool("gsdf.requiregpu", false, "Fail tests if no GPU device is available.")
)

// TestMain runs the GPU tests before running the CPU tests. GPU tests are skipped if no GPU device is available.
func TestMain(m *testing.M) {
	flag.Parse()
	var exit int
	err := testGsdfGPU()
	if err != nil {
		exit = 1
		log.Println(err)
	}
	os.Exit(m.Run() | exit)
}

func testGsdfGPU() error {
	var dev gpusdf.Device
	err := dev.Configure(gpusdf.DeviceConfig{ForceFallbackAdapter: *flagFallbackGPU})
	if err != nil {
		if *flagRequireGPU {
			return err
		}
		log.Println("skipping GPU tests, unable to configure GPU device:", err)
		return nil
	}
	defer dev.Release()
	log.Println("running GPU tests on", dev.AdapterInfo())
	cfg := newShaderTestConfig()
	cfg.dev = &dev
	cfg.prog.SetComputeInvocations(min(256, dev.MaxWorkgroupSize()), 1, 1)
	cfg.useGPU = true
	defer cfg.gpu3.Release()
	defer cfg.gpu2.Release()
	err = testGSDF(cfg)
	if cfg.failedObj != nil && *flagUIOnFail {
		gsdfaux.UI(cfg.failedObj, gsdfaux.UIConfig{Width: 800, Height: 600})
	}
	return err
}
