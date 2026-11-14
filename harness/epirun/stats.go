package main

import (
	"math"
	"math/rand/v2"
	"sort"
)

// z95 is the two-sided 95% normal quantile used for Wilson intervals.
const z95 = 1.959963984540054

// wilson95 returns the Wilson score 95% interval for x successes in n trials.
// For n = 0 it returns (0, 1).
func wilson95(x, n int) (lo, hi float64) {
	if n <= 0 {
		return 0, 1
	}
	nf := float64(n)
	p := float64(x) / nf
	z2 := z95 * z95
	den := 1 + z2/nf
	centre := (p + z2/(2*nf)) / den
	half := z95 * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / den
	lo, hi = centre-half, centre+half
	if x == 0 {
		lo = 0
	}
	if x == n {
		hi = 1
	}
	return math.Max(lo, 0), math.Min(hi, 1)
}

// logChoose is log C(n, k).
func logChoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

// binomPMF is P(X = k) for X ~ Binomial(n, p).
func binomPMF(k, n int, p float64) float64 {
	if k < 0 || k > n {
		return 0
	}
	switch {
	case p <= 0:
		if k == 0 {
			return 1
		}
		return 0
	case p >= 1:
		if k == n {
			return 1
		}
		return 0
	}
	return math.Exp(logChoose(n, k) + float64(k)*math.Log(p) + float64(n-k)*math.Log1p(-p))
}

// binomCDF is P(X <= x) for X ~ Binomial(n, p).
func binomCDF(x, n int, p float64) float64 {
	if x < 0 {
		return 0
	}
	if x >= n {
		return 1
	}
	s := 0.0
	for k := 0; k <= x; k++ {
		s += binomPMF(k, n, p)
	}
	return math.Min(s, 1)
}

// clopperPearsonUpper95 is the exact one-sided 95% upper limit for a
// proportion with x events in n trials: the p at which P(X <= x) falls to
// 0.05. For x = 0 and n = 40 it is 1 - 0.05^(1/40), about 7.2%. For n = 0 it
// is 1; for x = n it is 1.
func clopperPearsonUpper95(x, n int) float64 {
	if n <= 0 || x >= n {
		return 1
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if binomCDF(x, n, mid) > 0.05 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// mcnemarOneSided is the exact one-sided McNemar p-value for b pairs that
// moved in the hypothesised direction and c that moved the other way: the
// probability of b or more of b+c under a fair coin. With b + c = 0 it is 1.
func mcnemarOneSided(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1
	}
	s := 0.0
	for k := b; k <= n; k++ {
		s += binomPMF(k, n, 0.5)
	}
	return math.Min(s, 1)
}

// holmAdjust returns Holm-adjusted p-values, in the order of the input.
func holmAdjust(p []float64) []float64 {
	m := len(p)
	idx := make([]int, m)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return p[idx[a]] < p[idx[b]] })
	out := make([]float64, m)
	prev := 0.0
	for rank, i := range idx {
		adj := math.Min(1, p[i]*float64(m-rank))
		if adj < prev {
			adj = prev
		}
		out[i] = adj
		prev = adj
	}
	return out
}

// halfUp rounds to a whole number with halves going up (towards +infinity):
// 2.5 gives 3 and -2.5 gives -2.
func halfUp(f float64) float64 { return math.Floor(f + 0.5) }

// caseValue is one model's score on one case, for the bootstrap.
type caseValue struct {
	Case string
	V    float64
}

// panelBootstrap is the one-sided seeded bootstrap for "the panel's mean
// value is above zero", resampling cases with replacement. byModel maps each
// model to its per-case values. In every draw the same resampled list of
// cases is used for every model; each model's draw mean is the mean over its
// resampled cases that it has a value for, and the panel statistic is the
// plain mean of the model means, so every model counts equally however many
// cases it has. The p-value is (1 + draws with statistic <= 0) / (draws + 1);
// lower5 is the 5th percentile of the draws, a one-sided 95% lower bound.
// observed is the statistic on the original cases. Draws in which no model has
// a value are not counted. The same seed gives the same answer.
func panelBootstrap(byModel map[string][]caseValue, draws int, seed uint64) (observed, p, lower5 float64, valid int) {
	names := make([]string, 0, len(byModel))
	caseSet := map[string]bool{}
	for n, vs := range byModel {
		names = append(names, n)
		for _, v := range vs {
			caseSet[v.Case] = true
		}
	}
	sort.Strings(names)
	cases := make([]string, 0, len(caseSet))
	for c := range caseSet {
		cases = append(cases, c)
	}
	sort.Strings(cases)
	if len(cases) == 0 {
		return math.NaN(), math.NaN(), math.NaN(), 0
	}
	// idx[model][case] = value
	idx := make(map[string]map[string]float64, len(names))
	for _, n := range names {
		idx[n] = map[string]float64{}
		for _, v := range byModel[n] {
			idx[n][v.Case] = v.V
		}
	}
	stat := func(sample []string) (float64, bool) {
		sum, k := 0.0, 0
		for _, n := range names {
			s, c := 0.0, 0
			for _, cs := range sample {
				if v, ok := idx[n][cs]; ok {
					s += v
					c++
				}
			}
			if c > 0 {
				sum += s / float64(c)
				k++
			}
		}
		if k == 0 {
			return 0, false
		}
		return sum / float64(k), true
	}
	observed, _ = stat(cases)
	rng := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	vals := make([]float64, 0, draws)
	sample := make([]string, len(cases))
	atOrBelow := 0
	for d := 0; d < draws; d++ {
		for i := range sample {
			sample[i] = cases[rng.IntN(len(cases))]
		}
		s, ok := stat(sample)
		if !ok {
			continue
		}
		vals = append(vals, s)
		if s <= 0 {
			atOrBelow++
		}
	}
	valid = len(vals)
	if valid == 0 {
		return observed, math.NaN(), math.NaN(), 0
	}
	sort.Float64s(vals)
	p = float64(1+atOrBelow) / float64(valid+1)
	lower5 = vals[int(math.Floor(0.05*float64(valid-1)))]
	return observed, p, lower5, valid
}

// signFlipP is a one-sided sign-flip permutation test for "the mean of d is
// above zero". Each draw flips the sign of every d_i independently with
// probability 1/2; the p-value is (1 + draws with mean >= observed) / (draws +
// 1), so it is never 0. A d_i of 0 is unaffected by flipping. If every d_i is
// 0 (or d is empty) the observed mean is 0, every draw ties it, and p is 1.
// The same seed gives the same answer. observed is the mean of d (0 if empty).
func signFlipP(d []float64, draws int, seed uint64) (observed, p float64) {
	if len(d) == 0 {
		return 0, 1
	}
	n := float64(len(d))
	sum := 0.0
	for _, v := range d {
		sum += v
	}
	observed = sum / n
	rng := rand.New(rand.NewPCG(seed, 0x51ed270b1c9d6a3f))
	const tol = 1e-12 // sums of the same numbers in a different order differ by rounding
	ge := 0
	for i := 0; i < draws; i++ {
		s := 0.0
		for _, v := range d {
			if rng.Uint64()&1 == 0 {
				s += v
			} else {
				s -= v
			}
		}
		if s/n >= observed-tol {
			ge++
		}
	}
	return observed, float64(1+ge) / float64(draws+1)
}
