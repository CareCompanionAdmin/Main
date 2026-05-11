package service

import (
	"math"
	"testing"
)

const floatTol = 1e-6

func almostEq(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestMean(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 2, 3, 4, 5}, 3},
		{[]float64{-1, 1}, 0},
	}
	for _, c := range cases {
		if got := Mean(c.in); !almostEq(got, c.want, floatTol) {
			t.Errorf("Mean(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestStdDev(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{[]float64{}, 0},
		{[]float64{5}, 0},
		{[]float64{2, 4, 4, 4, 5, 5, 7, 9}, 2.13809}, // sample SD
		{[]float64{1, 1, 1, 1}, 0},
	}
	for _, c := range cases {
		if got := StdDev(c.in); !almostEq(got, c.want, 1e-4) {
			t.Errorf("StdDev(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestZScore(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	// mean=5, sd≈2.138
	if got := ZScore(xs, 10); !almostEq(got, (10-5)/2.13809, 1e-4) {
		t.Errorf("ZScore for target 10 = %v", got)
	}
	if got := ZScore(xs, 5); !almostEq(got, 0, 1e-9) {
		t.Errorf("ZScore for target=mean should be 0, got %v", got)
	}
	// Zero SD case — safe return 0
	if got := ZScore([]float64{3, 3, 3}, 5); !almostEq(got, 0, 1e-9) {
		t.Errorf("ZScore with zero SD = %v, want 0", got)
	}
}

func TestRollingMean(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	got := RollingMean(xs, 3)
	want := []float64{1, 1.5, 2, 3, 4}
	for i := range got {
		if !almostEq(got[i], want[i], floatTol) {
			t.Errorf("RollingMean[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestLinearRegression_StrongPositiveTrend(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ys := []float64{2, 4, 6, 8, 10, 12, 14, 16, 18, 20} // y = 2x
	slope, intercept, rSq, pValue := LinearRegression(xs, ys)
	if !almostEq(slope, 2.0, 1e-9) {
		t.Errorf("slope = %v, want 2.0", slope)
	}
	if !almostEq(intercept, 0.0, 1e-9) {
		t.Errorf("intercept = %v, want 0", intercept)
	}
	if !almostEq(rSq, 1.0, 1e-9) {
		t.Errorf("rSquared = %v, want 1.0", rSq)
	}
	if pValue > 0.001 {
		t.Errorf("pValue should be <0.001 for perfect linear trend, got %v", pValue)
	}
}

func TestLinearRegression_NoTrend(t *testing.T) {
	// Random walk around 5 with no slope
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ys := []float64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
	slope, _, _, pValue := LinearRegression(xs, ys)
	if !almostEq(slope, 0, 1e-9) {
		t.Errorf("slope on flat data = %v, want 0", slope)
	}
	if pValue < 0.5 {
		t.Errorf("pValue on flat data should be high, got %v", pValue)
	}
}

func TestLinearRegression_SmallSample(t *testing.T) {
	slope, _, _, p := LinearRegression([]float64{1, 2}, []float64{2, 4})
	if slope != 0 || p != 1 {
		t.Errorf("n=2 should return (0, 1) for slope/p, got slope=%v p=%v", slope, p)
	}
}

func TestPearsonPValue(t *testing.T) {
	// Perfect correlation
	if p := PearsonPValue(1.0, 10); p > 1e-6 {
		t.Errorf("p for r=1 should be 0, got %v", p)
	}
	// No correlation
	if p := PearsonPValue(0.0, 100); !almostEq(p, 1.0, 1e-3) {
		t.Errorf("p for r=0 should be ~1.0, got %v", p)
	}
	// r=0.6 n=20 → known p ≈ 0.005
	if p := PearsonPValue(0.6, 20); p > 0.01 || p < 0.001 {
		t.Errorf("p for r=0.6 n=20 should be ~0.005, got %v", p)
	}
	// Small sample
	if p := PearsonPValue(0.5, 2); p != 1 {
		t.Errorf("p for n=2 should be 1, got %v", p)
	}
}

func TestBenjaminiHochberg(t *testing.T) {
	// Classic example: 4 tests at α=0.05
	// Sorted p-values: 0.005, 0.01, 0.04, 0.5
	// Thresholds:      0.0125, 0.025, 0.0375, 0.05
	// Largest k with p ≤ threshold: k=2 (p=0.01, threshold=0.025)
	// So first 2 (originally at positions 0 and 1) survive.
	p := []float64{0.005, 0.01, 0.04, 0.5}
	got := BenjaminiHochberg(p, 0.05)
	want := []bool{true, true, false, false}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("BH[%d] = %v, want %v (p=%v)", i, got[i], want[i], p[i])
		}
	}
}

func TestBenjaminiHochberg_AllSurvive(t *testing.T) {
	// All extremely small p-values
	p := []float64{0.0001, 0.0002, 0.0003, 0.0004}
	got := BenjaminiHochberg(p, 0.05)
	for i, g := range got {
		if !g {
			t.Errorf("BH[%d] should survive with p=%v", i, p[i])
		}
	}
}

func TestBenjaminiHochberg_NoneSurvive(t *testing.T) {
	p := []float64{0.5, 0.6, 0.7, 0.8}
	got := BenjaminiHochberg(p, 0.05)
	for i, g := range got {
		if g {
			t.Errorf("BH[%d] should NOT survive with p=%v", i, p[i])
		}
	}
}

func TestBenjaminiHochberg_Empty(t *testing.T) {
	got := BenjaminiHochberg([]float64{}, 0.05)
	if len(got) != 0 {
		t.Errorf("BH on empty input should return empty, got len=%d", len(got))
	}
}

func TestDetectChangePoint_ClearShift(t *testing.T) {
	// 10 days at mood 7±0.5, then 10 days at mood 3±0.5 — clear step change near k=10.
	// Adding small jitter so within-segment variance is nonzero and the
	// algorithm can compute pooled standard error properly.
	xs := []float64{7, 7.5, 6.5, 7, 7.5, 6.5, 7, 7.5, 6.5, 7,
		3, 3.5, 2.5, 3, 3.5, 2.5, 3, 3.5, 2.5, 3}
	k, score := DetectChangePoint(xs, 3, 2.0)
	if k < 8 || k > 12 {
		t.Errorf("expected change point near k=10, got k=%d (score=%v)", k, score)
	}
	if score < 5 {
		t.Errorf("expected high score (>5) for clear shift, got %v", score)
	}
}

func TestDetectChangePoint_NoShift(t *testing.T) {
	xs := []float64{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7}
	k, _ := DetectChangePoint(xs, 3, 2.0)
	if k != -1 {
		t.Errorf("expected no change point for flat data, got k=%d", k)
	}
}

func TestDetectChangePoint_BelowThreshold(t *testing.T) {
	// Noisy data with no meaningful shift — within-segment variance is
	// large enough that a small mean drift shouldn't cross the 2.0 z-score gate.
	xs := []float64{5, 3, 7, 4, 6, 5, 5, 4, 6, 5, 5, 6, 4, 5, 6, 5, 4, 5, 6, 5}
	k, score := DetectChangePoint(xs, 3, 2.0)
	if k != -1 {
		t.Errorf("expected no change point below threshold, got k=%d (score=%v)", k, score)
	}
}

func TestDetectChangePoint_TooShort(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	k, _ := DetectChangePoint(xs, 3, 2.0)
	if k != -1 {
		t.Errorf("expected -1 for too-short input, got k=%d", k)
	}
}
