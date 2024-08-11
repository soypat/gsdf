package gleval

import (
	"errors"
	"fmt"

	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
)

// SDF3 implements a 3D signed distance field in vectorized
// form suitable for running on GPU.
type SDF3 interface {
	// Evaluate evaluates the signed distance field over pos positions.
	// dist and pos must be of same length.  Resulting distances are stored
	// in dist.
	//
	// userData facilitates getting data to the evaluators for use in processing, such as [VecPool].
	Evaluate(pos []ms3.Vec, dist []float32, userData any) error
	// Bounds returns the SDF's bounding box such that all of the shape is contained within.
	Bounds() ms3.Box
}

// SDF2 implements a 2D signed distance field in vectorized
// form suitable for running on GPU.
type SDF2 interface {
	// Evaluate evaluates the signed distance field over pos positions.
	// dist and pos must be of same length.  Resulting distances are stored
	// in dist.
	//
	// userData facilitates getting data to the evaluators for use in processing, such as [VecPool].
	Evaluate(pos []ms2.Vec, dist []float32, userData any) error
	// Bounds returns the SDF's bounding box such that all of the shape is contained within.
	Bounds() ms2.Box
}

// These interfaces are implemented by all SDF interfaces such as SDF3/2 and Shader3D/2D.
// Using these instead of `any` Aids in catching mistakes at compile time such as passing a Shader3D instead of Shader2D as an argument.
type (
	bounder2 = interface{ Bounds() ms2.Box }
	bounder3 = interface{ Bounds() ms3.Box }
)

// NormalsCentralDiff uses central differences algorithm for normal calculation, which are stored in normals for each position.
// The returned normals are not normalized (converted to unit length).
func NormalsCentralDiff(s SDF3, pos []ms3.Vec, normals []ms3.Vec, step float32, userData any) error {
	step *= 0.5
	if step <= 0 {
		return errors.New("invalid step")
	} else if len(pos) != len(normals) {
		return errors.New("length of position must match length of normals")
	} else if s == nil {
		return errors.New("nil SDF3")
	}
	vp, err := GetVecPool(userData)
	if err != nil {
		return fmt.Errorf("VecPool required in both GPU and CPU situations for Normal calculation: %s", err)
	}
	d1 := vp.Float.Acquire(len(pos))
	d2 := vp.Float.Acquire(len(pos))
	auxPos := vp.V3.Acquire(len(pos))
	defer vp.Float.Release(d1)
	defer vp.Float.Release(d2)
	defer vp.V3.Release(auxPos)
	var vecs = [3]ms3.Vec{{X: step}, {Y: step}, {Z: step}}
	for dim := 0; dim < 3; dim++ {
		h := vecs[dim]
		for i, p := range pos {
			auxPos[i] = ms3.Add(p, h)
		}
		err = s.Evaluate(auxPos, d1, userData)
		if err != nil {
			return err
		}
		for i, p := range pos {
			auxPos[i] = ms3.Sub(p, h)
		}
		err = s.Evaluate(auxPos, d2, userData)
		if err != nil {
			return err
		}
		switch dim {
		case 0:
			for i := range normals {
				normals[i].X = d1[i] - d2[i]
			}
		case 1:
			for i := range normals {
				normals[i].Y = d1[i] - d2[i]
			}
		case 2:
			for i := range normals {
				normals[i].Z = d1[i] - d2[i]
			}
		}
	}
	return nil
}
