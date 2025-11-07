package glrender

import (
	"io"

	"github.com/soypat/geometry/ms3"
)

const sqrt3 = 1.73205080757

type Renderer interface {
	ReadTriangles(dst []ms3.Triangle, userData any) (n int, err error)
}

// RenderAll reads the full contents of a Renderer and returns the slice read.
// It does not return error on io.EOF, like the io.RenderAll implementation.
func RenderAll(r Renderer, userData any) ([]ms3.Triangle, error) {
	const startSize = 4096
	var err error
	var nt int
	result := make([]ms3.Triangle, 0, startSize)
	buf := make([]ms3.Triangle, startSize)
	for {
		nt, err = r.ReadTriangles(buf, userData)
		if err == nil || err == io.EOF {
			result = append(result, buf[:nt]...)
		}
		if err != nil {
			break
		}
	}
	if err == io.EOF {
		return result, nil
	}
	return result, err
}

const minIcubeLvl = 1

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func aligndown(v, alignto int) int {
	return v &^ (alignto - 1)
}
