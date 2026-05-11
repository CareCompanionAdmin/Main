package service

// insight_statistics.go — pure statistical helpers used by the Phase 2
// internal-analysis layer (auto-correlation scanner, anomaly detector,
// trend detector, change-point detector).
//
// These functions are deliberately decoupled from models/repositories so
// they can be unit-tested in isolation against known inputs. Callers
// are responsible for assembling []float64 series from DataPoint slices.
//
// See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md

import (
	"math"
	"sort"
)

// Mean returns the arithmetic mean. Empty input returns 0.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// StdDev returns the sample standard deviation (N-1 denominator).
// Returns 0 for fewer than 2 values.
func StdDev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := Mean(xs)
	sumSq := 0.0
	for _, x := range xs {
		d := x - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(xs)-1))
}

// ZScore returns (target - mean) / stddev. If stddev is 0, returns 0
// (rather than NaN/Inf) so callers can compare safely.
func ZScore(xs []float64, target float64) float64 {
	sd := StdDev(xs)
	if sd == 0 {
		return 0
	}
	return (target - Mean(xs)) / sd
}

// RollingMean returns a slice of length len(xs) where each element is
// the mean of the trailing `window` values (inclusive of current).
// Indices before window-1 use whatever data is available so far.
func RollingMean(xs []float64, window int) []float64 {
	out := make([]float64, len(xs))
	if window < 1 {
		window = 1
	}
	for i := range xs {
		start := i - window + 1
		if start < 0 {
			start = 0
		}
		out[i] = Mean(xs[start : i+1])
	}
	return out
}

// LinearRegression fits y = slope*x + intercept by ordinary least squares.
// Returns (slope, intercept, rSquared, pValue). The p-value tests the
// null hypothesis slope==0 using a two-tailed t-test on the slope
// coefficient with N-2 degrees of freedom.
//
// Inputs of length < 3 return (0, 0, 0, 1) — not enough degrees of
// freedom to assess significance.
//
// pValue uses a Student's-t survival function approximation that is
// good enough for our N<200 use case (we're not trying to certify
// six-sigma confidence; we're flagging clinically interesting trends).
func LinearRegression(xs, ys []float64) (slope, intercept, rSq, pValue float64) {
	n := len(xs)
	if n != len(ys) || n < 3 {
		return 0, 0, 0, 1
	}
	mx := Mean(xs)
	my := Mean(ys)
	var sxx, sxy, syy float64
	for i := range xs {
		dx := xs[i] - mx
		dy := ys[i] - my
		sxx += dx * dx
		sxy += dx * dy
		syy += dy * dy
	}
	if sxx == 0 {
		return 0, my, 0, 1
	}
	slope = sxy / sxx
	intercept = my - slope*mx

	if syy > 0 {
		rSq = (sxy * sxy) / (sxx * syy)
		if rSq > 1 {
			rSq = 1
		}
	}

	// Standard error of the slope:
	//   SE = sqrt( (1 - rSq) * syy / ((n-2) * sxx) )
	if n > 2 && sxx > 0 && rSq < 1 {
		residualVar := (1 - rSq) * syy / float64(n-2)
		se := math.Sqrt(residualVar / sxx)
		if se > 0 {
			tStat := slope / se
			pValue = studentTSurvival(math.Abs(tStat), n-2) * 2 // two-tailed
		} else if math.Abs(slope) < 1e-12 {
			// Slope is exactly zero AND there's no variance left to test
			// against — interpret as "no trend detected" so the downstream
			// "slope != 0 && p < α" gate doesn't fire on flat input.
			pValue = 1
		} else {
			pValue = 0 // slope is exactly known and nonzero
		}
	} else if rSq >= 1 {
		pValue = 0 // perfect fit
	} else {
		pValue = 1
	}
	return
}

// PearsonPValue converts a correlation coefficient r and sample size n
// to a two-tailed p-value testing the null r==0. Returns 1 for invalid
// inputs (n < 3).
func PearsonPValue(r float64, n int) float64 {
	if n < 3 {
		return 1
	}
	rAbs := math.Abs(r)
	if rAbs >= 1 {
		return 0
	}
	// t = r * sqrt((n-2) / (1 - r^2))
	tStat := rAbs * math.Sqrt(float64(n-2)/(1-r*r))
	return studentTSurvival(tStat, n-2) * 2
}

// studentTSurvival returns P(T > t) for a Student's t distribution with
// df degrees of freedom and t >= 0. Uses the regularized incomplete
// beta function relationship:
//
//	P(T > t) = 0.5 * I(df/(df+t^2); df/2, 1/2)
//
// Implemented via a continued-fraction approximation that is accurate
// to ~1e-10 for our N<500, df>1 use case.
func studentTSurvival(t float64, df int) float64 {
	if t < 0 {
		return 1 - studentTSurvival(-t, df)
	}
	if df < 1 {
		return 0.5
	}
	x := float64(df) / (float64(df) + t*t)
	return 0.5 * regIncBeta(x, float64(df)/2, 0.5)
}

// regIncBeta is the regularized incomplete beta function I(x; a, b).
// Numerical Recipes-style continued-fraction implementation.
func regIncBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	bt := math.Exp(
		logGamma(a+b) - logGamma(a) - logGamma(b) +
			a*math.Log(x) + b*math.Log(1-x),
	)
	if x < (a+1)/(a+b+2) {
		return bt * betacf(x, a, b) / a
	}
	return 1 - bt*betacf(1-x, b, a)/b
}

func betacf(x, a, b float64) float64 {
	const eps = 1e-12
	const maxIters = 200
	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1.0 - qab*x/qap
	if math.Abs(d) < eps {
		d = eps
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIters; m++ {
		mF := float64(m)
		m2 := 2 * mF
		aa := mF * (b - mF) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < eps {
			d = eps
		}
		c = 1 + aa/c
		if math.Abs(c) < eps {
			c = eps
		}
		d = 1 / d
		h *= d * c
		aa = -(a + mF) * (qab + mF) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < eps {
			d = eps
		}
		c = 1 + aa/c
		if math.Abs(c) < eps {
			c = eps
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

// logGamma is math.Lgamma's first return.
func logGamma(x float64) float64 {
	g, _ := math.Lgamma(x)
	return g
}

// BenjaminiHochberg returns a boolean slice of the same length as
// pValues indicating which p-values survive the Benjamini-Hochberg FDR
// procedure at significance level alpha. Survivors are those for which
// the false discovery rate is controlled at alpha.
//
// This is the right correction for our exploratory factor-pair scan:
// we test hundreds of pairs and want to control the proportion of false
// "discoveries" rather than guarantee zero false positives (which
// Bonferroni would do at the cost of crushing legitimate findings).
func BenjaminiHochberg(pValues []float64, alpha float64) []bool {
	n := len(pValues)
	out := make([]bool, n)
	if n == 0 {
		return out
	}
	type idxP struct {
		idx int
		p   float64
	}
	sorted := make([]idxP, n)
	for i, p := range pValues {
		sorted[i] = idxP{i, p}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].p < sorted[j].p
	})
	// Find the largest k such that p_(k) <= k/n * alpha.
	cutoffK := -1
	for k, sp := range sorted {
		rank := k + 1
		threshold := float64(rank) / float64(n) * alpha
		if sp.p <= threshold {
			cutoffK = k
		}
	}
	if cutoffK < 0 {
		return out
	}
	for k := 0; k <= cutoffK; k++ {
		out[sorted[k].idx] = true
	}
	return out
}

// DetectChangePoint scans a series for a single change point where the
// mean of values before and after differs most. Returns (index, score)
// where score is the absolute z-score of the difference under the
// assumption of pre-change variance. An index of -1 means no candidate
// was found.
//
// Algorithm: for each candidate split point k in [minSegment, n-minSegment),
// compute |mean_left - mean_right| / pooled_stddev. Return the k with
// the maximum score, gated by a minimum score threshold.
//
// Use case: catches medication-start effects, regression onsets,
// schedule-change impacts — "something shifted around Day-N".
func DetectChangePoint(xs []float64, minSegment int, scoreThreshold float64) (int, float64) {
	n := len(xs)
	if minSegment < 3 {
		minSegment = 3
	}
	if n < 2*minSegment {
		return -1, 0
	}
	bestK := -1
	bestScore := 0.0
	for k := minSegment; k <= n-minSegment; k++ {
		left := xs[:k]
		right := xs[k:]
		ml := Mean(left)
		mr := Mean(right)
		sl := StdDev(left)
		sr := StdDev(right)
		// Pooled standard error of the difference in means.
		nl := float64(len(left))
		nr := float64(len(right))
		pooledVar := ((nl-1)*sl*sl + (nr-1)*sr*sr) / (nl + nr - 2)
		se := math.Sqrt(pooledVar * (1/nl + 1/nr))
		if se == 0 {
			continue
		}
		score := math.Abs(ml-mr) / se
		if score > bestScore {
			bestScore = score
			bestK = k
		}
	}
	if bestScore < scoreThreshold {
		return -1, bestScore
	}
	return bestK, bestScore
}
