package gsdf

import (
	"math"

	"github.com/chewxy/math32"
	"github.com/soypat/glgl/math/ms1"
	"github.com/soypat/glgl/math/ms2"
	"github.com/soypat/glgl/math/ms3"
	"github.com/soypat/gsdf/gleval"
)

// minReduce takes element-wise minimum of arguments and stores to first argument.
func minReduce(d1AndDst, d2 []float32) {
	for i := range d1AndDst {
		d1AndDst[i] = math32.Min(d1AndDst[i], d2[i])
	}
}

func (u *sphere) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	r := u.r
	for i, p := range pos {
		dist[i] = ms3.Norm(p) - r
	}
	return nil
}

func (b *box) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	d := ms3.Scale(0.5, b.dims)
	r := b.round
	for i, p := range pos {
		q := ms3.AddScalar(r, ms3.Sub(ms3.AbsElem(p), d))
		dist[i] = ms3.Norm(ms3.MaxElem(q, ms3.Vec{})) + minf(maxf(q.X, maxf(q.Y, q.Z)), 0.0) - r
	}
	return nil
}

func (bf *boxframe) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	e, b := bf.args()
	var z3 ms3.Vec
	for i, p := range pos {
		p = ms3.Sub(ms3.AbsElem(p), b)
		q := ms3.AddScalar(-e, ms3.AbsElem(ms3.AddScalar(e, p)))

		s1 := math32.Min(0, math32.Max(p.X, math32.Max(q.Y, q.Z)))            // min(max(p.x,max(q.y,q.z)),0.0)
		n1 := ms3.Norm(ms3.MaxElem(ms3.Vec{X: p.X, Y: q.Y, Z: q.Z}, z3)) + s1 // length(max(vec3(p.x,q.y,q.z),0.0))+s1

		s2 := math32.Min(0, math32.Max(q.X, math32.Max(p.Y, q.Z)))            // min(max(q.x,max(p.y,q.z)),0.0)
		n2 := ms3.Norm(ms3.MaxElem(ms3.Vec{X: q.X, Y: p.Y, Z: q.Z}, z3)) + s2 // length(max(vec3(q.x,p.y,q.z),0.0))+s2

		s3 := math32.Min(0, math32.Max(q.X, math32.Max(q.Y, p.Z)))            // min(max(q.x,max(q.y,p.z)),0.0))
		n3 := ms3.Norm(ms3.MaxElem(ms3.Vec{X: q.X, Y: q.Y, Z: p.Z}, z3)) + s3 // length(max(vec3(q.x,q.y,p.z),0.0))+s3

		dist[i] = math32.Min(n1, math32.Min(n2, n3))
	}
	return nil
}

func (t *torus) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	t1 := t.rGreater
	t2 := t.rLesser
	for i, p := range pos {
		p = ms3.Vec{X: p.X, Y: p.Z, Z: p.Y}
		q := ms2.Vec{X: hypotf(p.X, p.Z) - t1, Y: p.Y}
		dist[i] = ms2.Norm(q) - t2
	}
	return nil
}

func (c *cylinder) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	r, h, round := c.args()
	if round == 0 {
		for i, p := range pos {
			p = ms3.Vec{X: p.X, Y: p.Z, Z: p.Y}
			dx := math32.Hypot(p.X, p.Z) - r
			dy := math32.Abs(p.Y) - h
			dist[i] = minf(0, maxf(dx, dy)) + math32.Hypot(math32.Max(0, dx), math32.Max(0, dy))
		}
	} else {
		for i, p := range pos {
			p = ms3.Vec{X: p.X, Y: p.Z, Z: p.Y}
			dx := math32.Hypot(p.X, p.Z) - r + round
			dy := math32.Abs(p.Y) - h
			dist[i] = minf(maxf(dx, dy), 0) + hypotf(maxf(dx, 0), maxf(dy, 0)) - round
		}
	}
	return nil
}

func (h *hex) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	const k1, k2, k3 = -tribisect, 0.5, 0.57735
	h1 := h.side
	h2 := h.h
	clm := k3 * h1
	for i, p := range pos {
		p = ms3.AbsElem(p)
		pm := minf(k1*p.X+k2*p.Y, 0)
		p.X -= 2 * k1 * pm
		p.Y -= 2 * k2 * pm
		d1 := hypotf(p.X-clampf(p.X, -clm, clm), p.Y-h1) * signf(p.Y-h1)
		d2 := p.Z - h2
		dist[i] = minf(maxf(d1, d2), 0) + hypotf(maxf(d1, 0), maxf(d2, 0))
	}
	return nil
}

func evaluateSDF3(obj bounder3, pos []ms3.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF3(obj)
	if err != nil {
		return err
	}
	return sdf.Evaluate(pos, dist, userData)
}

func evaluateSDF2(obj bounder2, pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(obj)
	if err != nil {
		return err
	}
	return sdf.Evaluate(pos, dist, userData)
}

// Evaluate implements [gleval.SDF3].
func (u *OpUnion) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	u.mustValidate()
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	auxDist := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(auxDist)
	err = evaluateSDF3(u.joined[0], pos, dist, userData)
	if err != nil {
		return err
	}
	for _, shape := range u.joined[1:] {
		err = evaluateSDF3(shape, pos, auxDist, userData)
		if err != nil {
			return err
		}
		minReduce(dist, auxDist)
	}
	return nil
}

func (u *intersect) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF3(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF3(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	for i := range d1 {
		dist[i] = maxf(d1[i], d2[i])
	}
	return nil
}

func (u *diff) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF3(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF3(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	for i := range dist {
		dist[i] = maxf(d1[i], -d2[i])
	}
	return nil
}

func (u *xor) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF3(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF3(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	for i := range dist {
		a, b := d1[i], d2[i]
		dist[i] = maxf(minf(a, b), -maxf(a, b))
	}
	return nil
}

func (u *smoothUnion) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF3(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF3(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	k := u.k
	for i := range dist {
		a, b := d1[i], d2[i]
		h := clampf(0.5+0.5*(b-a)/k, 0, 1)
		dist[i] = mixf(b, a, h) - k*h*(1-h)
	}
	return nil
}

func (u *smoothDiff) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF3(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF3(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	k := u.k
	for i := range dist {
		a, b := d1[i], d2[i]
		h := clampf(0.5-0.5*(b+a)/k, 0, 1)
		dist[i] = mixf(a, -b, h) + k*h*(1-h)
	}
	return nil
}

func (u *smoothIntersect) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF3(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF3(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	k := u.k
	for i := range dist {
		a, b := d1[i], d2[i]
		h := clampf(0.5-0.5*(b-a)/k, 0, 1)
		dist[i] = mixf(b, a, h) + k*h*(1-h)
	}
	return nil
}

func (s *scale) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF3(s.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	scaled := vp.V3.Acquire(len(pos))
	defer vp.V3.Release(scaled)
	factor := s.scale
	factorInv := 1. / s.scale
	for i, p := range pos {
		scaled[i] = ms3.Scale(factorInv, p)
	}
	err = sdf.Evaluate(scaled, dist, userData)
	if err != nil {
		return err
	}
	for i := range dist {
		dist[i] *= factor
	}
	return nil
}

func (s *symmetry) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF3(s.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V3.Acquire(len(pos))
	copy(transformed, pos)
	defer vp.V3.Release(transformed)
	xb, yb, zb := s.xyz.X(), s.xyz.Y(), s.xyz.Z()
	for i, p := range transformed {
		if xb {
			transformed[i].X = absf(p.X)
		}
		if yb {
			transformed[i].Y = absf(p.Y)
		}
		if zb {
			transformed[i].Z = absf(p.Z)
		}
	}
	err = sdf.Evaluate(transformed, dist, userData)
	if err != nil {
		return err
	}
	return nil
}

func (a *array) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF3(a.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V3.Acquire(len(pos))
	defer vp.V3.Release(transformed)
	auxdist := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(auxdist)
	s := a.d
	n := ms3.AddScalar(-1, a.nvec3())
	minlim := ms3.Vec{}
	_ = n
	_ = minlim
	for i := range dist {
		dist[i] = largenum
	}
	// We invert loops with respect to shader here to avoid needing 8 distance and 8 position buffers, instead we need 1 of each with this loop shape.
	var ijk ms3.Vec
	for k := float32(0.); k < 2; k++ {
		ijk.Z = k
		for j := float32(0.); j < 2; j++ {
			ijk.Y = j
			for i := float32(0.); i < 2; i++ {
				ijk.X = i
				// We acquire the transformed position for each direction.
				for ip, p := range pos {
					id := ms3.RoundElem(ms3.DivElem(p, s))
					o := ms3.SignElem(ms3.Sub(p, ms3.MulElem(s, id)))

					rid := ms3.Add(id, ms3.MulElem(ijk, o))
					rid = ms3.ClampElem(rid, minlim, n)

					transformed[ip] = ms3.Sub(p, ms3.MulElem(s, rid))
				}
				// And calculate the distance for each direction.
				err := sdf.Evaluate(transformed, auxdist, userData)
				if err != nil {
					return err
				}
				// And we reduce the distance with minimum rule.
				for i, d := range dist {
					dist[i] = minf(d, auxdist[i])
				}
			}
		}
	}
	return nil
}

func (e *elongate) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF3(e.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V3.Acquire(len(pos))
	defer vp.V3.Release(transformed)
	aux := vp.Float.Acquire(len(pos))
	defer vp.Float.Release(aux)
	h := ms3.Scale(0.5, e.h)
	for i, p := range pos {
		q := ms3.Sub(ms3.AbsElem(p), h)
		aux[i] = math32.Min(q.Max(), 0)
		transformed[i] = ms3.MaxElem(q, ms3.Vec{})
	}
	err = sdf.Evaluate(transformed, dist, userData)
	if err != nil {
		return err
	}
	for i, qnorm := range aux {
		dist[i] += qnorm
	}
	return nil
}

func (sh *shell) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	auxpos := vp.V3.Acquire(len(pos))
	defer vp.V3.Release(auxpos)
	thickness := sh.thick
	for i, p := range pos {
		auxpos[i] = ms3.Scale(1/thickness, p)
	}
	sdf, err := gleval.AssertSDF3(sh.s)
	if err != nil {
		return err
	}
	err = sdf.Evaluate(auxpos, dist, userData)
	if err != nil {
		return err
	}

	for i, d := range dist {
		dist[i] = thickness * (absf(d) - thickness)
	}
	return nil
}

func (r *offset) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF3(r.s)
	if err != nil {
		return err
	}
	err = sdf.Evaluate(pos, dist, userData)
	if err != nil {
		return err
	}
	radius := r.off
	for i, d := range dist {
		dist[i] = d + radius
	}
	return nil
}

func (t *translate) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V3.Acquire(len(pos))
	defer vp.V3.Release(transformed)
	T := t.p
	for i, p := range pos {
		transformed[i] = ms3.Sub(p, T)
	}
	sdf, err := gleval.AssertSDF3(t.s)
	if err != nil {
		return err
	}
	return sdf.Evaluate(transformed, dist, userData)
}

func (t *transform) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V3.Acquire(len(pos))
	defer vp.V3.Release(transformed)
	Tinv := t.tInv
	for i, p := range pos {
		transformed[i] = Tinv.MulPosition(p)
	}
	sdf, err := gleval.AssertSDF3(t.s)
	if err != nil {
		return err
	}
	return sdf.Evaluate(transformed, dist, userData)
}

func (e *extrusion) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	sdf, err := gleval.AssertSDF2(e.s)
	if err != nil {
		return err
	}
	pos2 := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(pos2)
	for i, p := range pos {
		pos2[i] = ms2.Vec{X: p.X, Y: p.Y}
	}
	err = sdf.Evaluate(pos2, dist, userData)
	if err != nil {
		return err
	}
	h := e.h / 2
	for i, p := range pos {
		d := dist[i]
		wy := math32.Abs(p.Z) - h
		dist[i] = math32.Min(0, math32.Max(d, wy)) + math32.Hypot(math32.Max(d, 0), math32.Max(wy, 0))
	}
	return nil
}

func (e *revolution) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	o := e.off
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	sdf, err := gleval.AssertSDF2(e.s2d)
	if err != nil {
		return err
	}
	pos2 := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(pos2)
	for i, p := range pos {
		pos2[i] = ms2.Vec{X: math32.Hypot(p.X, p.Z) - o, Y: p.Y}
	}
	return sdf.Evaluate(pos2, dist, userData)
}

func (l *line2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	a := l.a
	ba := ms2.Sub(l.b, l.a)
	dotba := ms2.Dot(ba, ba)
	w := l.width / 2
	for i, p := range pos {
		pa := ms2.Sub(p, a)
		h := ms1.Clamp(ms2.Dot(pa, ba)/dotba, 0, 1)
		dist[i] = ms2.Norm(ms2.Sub(pa, ms2.Scale(h, ba))) - w
	}
	return nil
}

func (a *arc2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	r := a.radius
	t := a.thick / 2
	s, c := math32.Sincos(a.angle / 2)
	sc := ms2.Vec{X: s, Y: c}
	scr := ms2.Scale(r, sc)
	for i, p := range pos {
		p.X = math32.Abs(p.X)
		if sc.Y*p.X > sc.X*p.Y {
			dist[i] = ms2.Norm(ms2.Sub(p, scr)) - t
		} else {
			dist[i] = math32.Abs(ms2.Norm(p)-r) - t
		}
	}
	return nil
}

func (c *circle2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	r := c.r
	for i, p := range pos {
		dist[i] = ms2.Norm(p) - r
	}
	return nil
}

func (t *equilateralTri2d) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	const k = sqrt3
	r := t.hTri / sqrt3
	for i, p := range pos {
		p.X = math32.Abs(p.X) - r
		p.Y += r / k
		if p.X+k*p.Y > 0 {
			p = ms2.Vec{X: p.X - k*p.Y, Y: -k*p.X - p.Y}
			p = ms2.Scale(0.5, p)
		}
		p.X -= clampf(p.X, -2*r, 0)
		dist[i] = -ms2.Norm(p) * signf(p.Y)
	}
	return nil
}

func (c *rect2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	b := ms2.Scale(0.5, c.d)
	for i, p := range pos {
		d := ms2.Sub(ms2.AbsElem(p), b)
		dist[i] = ms2.Norm(ms2.MaxElem(d, ms2.Vec{})) + math32.Min(0, math32.Max(d.X, d.Y))
	}
	return nil
}

func (c *hex2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	r := c.side
	k := ms2.Vec{X: -tribisect, Y: 0.5}
	const kz = 0.577350269
	for i, p := range pos {
		p = ms2.AbsElem(p)
		p = ms2.Sub(p, ms2.Scale(2*math32.Min(ms2.Dot(k, p), 0), k))
		p = ms2.Sub(p, ms2.Vec{X: clampf(p.X, -kz*r, kz*r), Y: r})
		dist[i] = signf(p.Y) * ms2.Norm(p)
	}
	return nil
}

func (c *oct2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	const kx, ky, kz = -0.9238795325, 0.3826834323, 0.4142135623
	r := c.c
	kzr := kz * r
	nkzr := -kzr
	v1 := ms2.Vec{X: kx, Y: ky}
	v2 := ms2.Vec{X: -kx, Y: ky}
	clamp := ms2.Vec{Y: r}
	for i, p := range pos {
		p = ms2.AbsElem(p)
		p = ms2.Sub(p, ms2.Scale(2*math32.Min(ms2.Dot(v1, p), 0), v1))
		p = ms2.Sub(p, ms2.Scale(2*math32.Min(ms2.Dot(v2, p), 0), v2))
		clamp.X = ms1.Clamp(p.X, nkzr, kzr)
		p = ms2.Sub(p, clamp)
		dist[i] = signf(p.Y) * ms2.Norm(p)
	}
	return nil
}

func (c *ellipse2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	// https://iquilezles.org/articles/ellipsedist
	for i, p := range pos {
		a, b := c.a, c.b
		p = ms2.AbsElem(p)
		if p.X > p.Y {
			p.X, p.Y = p.Y, p.X
			a, b = b, a
		}
		l := b*b - a*a
		m := a * p.X / l
		m2 := m * m
		n := b * p.Y / l
		n2 := n * n
		c := (m2 + n2 - 1) / 3
		c3 := c * c * c
		q := c3 + 2*m2*n2
		d := c3 + m2*n2
		g := m + m*n2
		var co float32
		if d < 0 {
			h := math32.Acos(q/c3) / 3
			sh, ch := math32.Sincos(h)
			t := sqrt3 * sh
			rx := math32.Sqrt(-c*(ch+t+2) + m2)
			ry := math32.Sqrt(-c*(ch-t+2) + m2)
			co = (ry + signf(l)*rx + math32.Abs(g)/(rx*ry) - m) / 2
		} else {
			h := 2 * m * n * math32.Sqrt(d)
			s := signf(q+h) * math32.Cbrt(math32.Abs(q+h))
			u := signf(q-h) * math32.Cbrt(math32.Abs(q-h))

			rx := -s - u - 4*c + 2*m2
			ry := sqrt3 * (s - u)
			rm := math32.Hypot(rx, ry)
			co = (ry/math32.Sqrt(rm-rx) + 2*g/rm - m) / 2
		}
		r := ms2.Vec{X: a * co, Y: b * math32.Sqrt(1-co*co)}
		dist[i] = ms2.Norm(ms2.Sub(r, p)) * signf(p.Y-r.Y)
	}
	return nil
}

func (p *poly2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	// https://www.shadertoy.com/view/wdBXRW
	verts := p.vert
	for i, p := range pos {
		d := ms2.Norm2(ms2.Sub(p, verts[0]))
		s := float32(1.0)
		jv := len(verts) - 1
		for iv, v1 := range verts {
			v2 := verts[jv]
			e := ms2.Sub(v2, v1)
			w := ms2.Sub(p, v1)
			b := ms2.Sub(w, ms2.Scale(ms1.Clamp(ms2.Dot(w, e)/ms2.Norm2(e), 0, 1), e))
			d = math32.Min(d, ms2.Norm2(b))
			// winding number from http://geomalgorithms.com/a03-_inclusion.html
			b1 := p.Y >= v1.Y
			b2 := p.Y < v2.Y
			b3 := e.X*w.Y > e.Y*w.X
			if (b1 && b2 && b3) || ((!b1) && (!b2) && (!b3)) {
				s = -s
			}
			jv = iv
		}
		dist[i] = s * math32.Sqrt(d)
	}
	return nil
}

// Evaluate implements [gleval.SDF2].
func (u *OpUnion2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	// Same algorithm as UnionOp.Evaluate. see that method for commentary.
	u.mustValidate()
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	auxDist := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(auxDist)

	err = evaluateSDF2(u.joined[0], pos, dist, userData)
	if err != nil {
		return err
	}
	for _, shape := range u.joined[1:] {
		err = evaluateSDF2(shape, pos, auxDist, userData)
		if err != nil {
			return err
		}
		for i, d := range dist {
			dist[i] = math32.Min(d, auxDist[i])
		}
	}
	return nil
}

func (u *intersect2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF2(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF2(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	for i := range dist {
		dist[i] = maxf(d1[i], d2[i])
	}
	return nil
}

func (u *diff2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF2(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF2(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	for i := range dist {
		dist[i] = maxf(d1[i], -d2[i])
	}
	return nil
}

func (u *xor2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	d1 := dist
	d2 := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(d2)
	err = evaluateSDF2(u.s1, pos, d1, userData)
	if err != nil {
		return err
	}
	err = evaluateSDF2(u.s2, pos, d2, userData)
	if err != nil {
		return err
	}
	for i := range dist {
		a, b := d1[i], d2[i]
		dist[i] = maxf(minf(a, b), -maxf(a, b))
	}
	return nil
}

func (a *array2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(a.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(transformed)
	auxdist := vp.Float.Acquire(len(dist))
	defer vp.Float.Release(auxdist)
	s := a.d
	n := ms2.AddScalar(-1, a.nvec2())
	minlim := ms2.Vec{}

	for i := range dist {
		dist[i] = largenum
	}
	// We invert loops with respect to shader here to avoid needing 8 distance and 8 position buffers, instead we need 1 of each with this loop shape.
	var ij ms2.Vec
	for j := float32(0.); j < 2; j++ {
		ij.Y = j
		for i := float32(0.); i < 2; i++ {
			ij.X = i
			// We acquire the transformed position for each direction.
			for ip, p := range pos {
				id := ms2.RoundElem(ms2.DivElem(p, s))
				o := ms2.SignElem(ms2.Sub(p, ms2.MulElem(s, id)))

				rid := ms2.Add(id, ms2.MulElem(ij, o))
				rid = ms2.ClampElem(rid, minlim, n)

				transformed[ip] = ms2.Sub(p, ms2.MulElem(s, rid))
			}
			// And calculate the distance for each direction.
			err := sdf.Evaluate(transformed, auxdist, userData)
			if err != nil {
				return err
			}
			// And we reduce the distance with minimum rule.
			for i, d := range dist {
				dist[i] = minf(d, auxdist[i])
			}
		}
	}
	return nil
}

func (r *offset2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(r.s)
	if err != nil {
		return err
	}
	err = sdf.Evaluate(pos, dist, userData)
	if err != nil {
		return err
	}
	radius := r.f
	for i, d := range dist {
		dist[i] = d + radius
	}
	return nil
}

func (t *translate2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(t.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(transformed)
	T := t.p
	for i, p := range pos {
		transformed[i] = ms2.Sub(p, T)
	}
	return sdf.Evaluate(transformed, dist, userData)
}

func (s *symmetry2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(s.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V2.Acquire(len(pos))
	copy(transformed, pos)
	defer vp.V2.Release(transformed)
	xb, yb := s.xy.X(), s.xy.Y()
	for i, p := range transformed {
		if xb {
			transformed[i].X = absf(p.X)
		}
		if yb {
			transformed[i].Y = absf(p.Y)
		}
	}
	err = sdf.Evaluate(transformed, dist, userData)
	if err != nil {
		return err
	}
	return nil
}

func (s *annulus2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(s.s)
	if err != nil {
		return err
	}
	err = sdf.Evaluate(pos, dist, userData)
	if err != nil {
		return err
	}
	r := s.r
	for i, d := range dist {
		dist[i] = math32.Abs(d) - r
	}
	return nil
}

func (c *circarray) Evaluate(pos []ms3.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	sdf, err := gleval.AssertSDF3(c.s)
	if err != nil {
		return err
	}
	pos0 := vp.V3.Acquire(len(pos))
	pos1 := vp.V3.Acquire(len(pos))
	defer vp.V3.Release(pos0)
	defer vp.V3.Release(pos1)

	angle := 2 * math32.Pi / float32(c.circleDiv)
	ncirc := float32(c.circleDiv)
	ninsm1 := float32(c.nInst - 1)
	for i, p := range pos {
		pangle := math32.Atan2(p.Y, p.X)
		id := math32.Floor(pangle / angle)
		if id < 0 {
			id += ncirc
		}
		var i0, i1 float32
		if id >= ninsm1 {
			i0 = ninsm1
			i1 = 0
		} else {
			i0 = id
			i1 = id + 1
		}
		p2d := ms2.Vec{X: p.X, Y: p.Y}
		p0 := ms2.MulMatVecTrans(ms2.RotationMat2(angle*i0), p2d)
		p1 := ms2.MulMatVecTrans(ms2.RotationMat2(angle*i1), p2d)
		pos0[i] = ms3.Vec{X: p0.X, Y: p0.Y, Z: p.Z}
		pos1[i] = ms3.Vec{X: p1.X, Y: p1.Y, Z: p.Z}
	}

	dist1 := vp.Float.Acquire(len(pos))
	defer vp.Float.Release(dist1)
	err = sdf.Evaluate(pos1, dist1, userData)
	if err != nil {
		return err
	}
	err = sdf.Evaluate(pos0, dist, userData)
	if err != nil {
		return err
	}
	minReduce(dist, dist1)
	return nil
}

func (c *circarray2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	sdf, err := gleval.AssertSDF2(c.s)
	if err != nil {
		return err
	}
	pos0 := vp.V2.Acquire(len(pos))
	pos1 := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(pos0)
	defer vp.V2.Release(pos1)

	angle := 2 * math32.Pi / float32(c.circleDiv)
	ncirc := float32(c.circleDiv)
	ninsm1 := float32(c.nInst - 1)
	for i, p := range pos {
		pangle := math32.Atan2(p.Y, p.X)
		id := math32.Floor(pangle / angle)
		if id < 0 {
			id += ncirc
		}
		var i0, i1 float32
		if id >= ninsm1 {
			i0 = ninsm1
			i1 = 0
		} else {
			i0 = id
			i1 = id + 1
		}
		pos0[i] = ms2.MulMatVecTrans(ms2.RotationMat2(angle*i0), p)
		pos1[i] = ms2.MulMatVecTrans(ms2.RotationMat2(angle*i1), p)
	}

	dist1 := vp.Float.Acquire(len(pos))
	defer vp.Float.Release(dist1)
	err = sdf.Evaluate(pos1, dist1, userData)
	if err != nil {
		return err
	}
	err = sdf.Evaluate(pos0, dist, userData)
	if err != nil {
		return err
	}
	for i, d := range dist {
		dist[i] = math32.Min(d, dist1[i])
	}
	return nil
}

func (l *lines2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	w := l.width / 2
	for i, p := range pos {
		d := float32(1e23)
		for _, v1v2 := range l.points {
			a, b := v1v2[0], v1v2[1]
			pa := ms2.Sub(p, a)
			ba := ms2.Sub(b, a)
			dotba := ms2.Dot(ba, ba)
			h := ms1.Clamp(ms2.Dot(pa, ba)/dotba, 0, 1)
			d = math32.Min(d, ms2.Norm2(ms2.Sub(pa, ms2.Scale(h, ba))))
		}
		dist[i] = math32.Sqrt(d) - w
	}
	return nil
}

func (c *translateMulti2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	for i := range dist {
		dist[i] = math.MaxFloat32
	}
	d1 := vp.Float.Acquire(len(pos))
	defer vp.Float.Release(d1)
	for _, p := range c.displacements {
		t2d := translate2D{
			s: c.s,
			p: p,
		}
		err = t2d.Evaluate(pos, d1, userData)
		if err != nil {
			return err
		}
		minReduce(dist, d1)
	}
	return nil
}

func (c *rotation2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(c.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	posTransf := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(posTransf)
	invT := c.tInv
	for i, p := range pos {
		posTransf[i] = ms2.MulMatVec(invT, p)
	}
	err = sdf.Evaluate(posTransf, dist, userData)
	return err
}

func (c *scale2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(c.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	posTransf := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(posTransf)
	invScale := 1. / c.scale
	for i, p := range pos {
		posTransf[i] = ms2.Scale(invScale, p)
	}
	err = sdf.Evaluate(posTransf, dist, userData)
	scale := c.scale
	for i, d := range dist {
		dist[i] = d * scale
	}
	return err
}

func (e *elongate2D) Evaluate(pos []ms2.Vec, dist []float32, userData any) error {
	sdf, err := gleval.AssertSDF2(e.s)
	if err != nil {
		return err
	}
	vp, err := gleval.GetVecPool(userData)
	if err != nil {
		return err
	}
	transformed := vp.V2.Acquire(len(pos))
	defer vp.V2.Release(transformed)
	aux := vp.Float.Acquire(len(pos))
	defer vp.Float.Release(aux)
	h := ms2.Scale(0.5, e.h)
	for i, p := range pos {
		q := ms2.Sub(ms2.AbsElem(p), h)
		aux[i] = math32.Min(q.Max(), 0)
		transformed[i] = ms2.MaxElem(q, ms2.Vec{})
	}
	err = sdf.Evaluate(transformed, dist, userData)
	if err != nil {
		return err
	}
	for i, qnorm := range aux {
		dist[i] += qnorm
	}
	return nil
}
