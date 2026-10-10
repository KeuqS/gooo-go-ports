package main

import (
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"time"
)

const durationPortBatchSize = 128
const durationQuantizerBatchSize = 64

type durationObservations struct {
	Suites                 []suite
	PortReferenceCases     int
	ConsumerReferenceCases int
}

func durationCorpus() (durationObservations, error) {
	result := durationObservations{}
	inputs := durationInputs()
	for _, operation := range []struct {
		pkg, activity string
		oracle        func(int64, int64) (any, error)
	}{
		{"go/time", "DurationRound", func(d, m int64) (any, error) { return int64(time.Duration(d).Round(time.Duration(m))), nil }},
		{"go/time", "DurationTruncate", func(d, m int64) (any, error) { return int64(time.Duration(d).Truncate(time.Duration(m))), nil }},
		{"app/duration-quantizer", "Inspect", durationQuantizerOracle},
	} {
		s, err := durationSuite(operation.pkg, operation.activity, inputs, operation.oracle)
		if err != nil {
			return durationObservations{}, err
		}
		s.Domain = fmt.Sprintf("signed int64 nanoseconds for both duration and quantum; %d finite Go-reference pairs covering a small grid, original int64 endpoints, nonpositive quanta, half thresholds, exact multiples, saturation transitions and fixed PCG samples; original inputs retained; no domain-rejection extension", len(s.Cases))
		if operation.pkg == "go/time" {
			result.PortReferenceCases += len(s.Cases)
		} else {
			result.ConsumerReferenceCases += len(s.Cases)
		}
		result.Suites = append(result.Suites, s)
	}
	return result, nil
}

// Validate the entire roster before any conversion or oracle call. Duration is
// int64 on all Go targets; neither argument is converted through a Go int.
func durationSuite(pkg, activity string, inputs [][]any, oracle func(int64, int64) (any, error)) (suite, error) {
	if len(inputs) == 0 {
		return suite{}, fmt.Errorf("%s: empty duration corpus", activity)
	}
	seen := map[[2]int64]bool{}
	for _, input := range inputs {
		if len(input) != 2 {
			return suite{}, fmt.Errorf("%s: expected duration and quantum", activity)
		}
		d, durationOK := input[0].(int64)
		m, quantumOK := input[1].(int64)
		if !durationOK || !quantumOK || seen[[2]int64{d, m}] {
			return suite{}, fmt.Errorf("%s: require distinct pairs of exact int64 values: %v", activity, input)
		}
		seen[[2]int64{d, m}] = true
	}
	s := suite{Package: pkg, Activity: activity}
	for _, input := range inputs {
		expected, err := oracle(input[0].(int64), input[1].(int64))
		if err != nil {
			return suite{}, err
		}
		s.Cases = append(s.Cases, testCase{Inputs: append([]any(nil), input...), Expected: expected})
	}
	return s, nil
}

func durationBoundaryValues() []int64 {
	return []int64{math.MinInt64, math.MinInt64 + 1, math.MinInt64 + 2,
		-9000000000000000000, -8000000000000000000, -4611686018427387905, -4611686018427387904, -4611686018427387903,
		-9007199254740993, -9007199254740992, -9007199254740991, math.MinInt32,
		-1, 0, 1, math.MaxInt32, 9007199254740991, 9007199254740992, 9007199254740993,
		4611686018427387903, 4611686018427387904, 4611686018427387905, 8000000000000000000, 9000000000000000000,
		math.MaxInt64 - 2, math.MaxInt64 - 1, math.MaxInt64}
}

func durationQuanta() []int64 {
	return []int64{math.MinInt64, math.MinInt64 + 1, -1000000000, -1, 0, 1, 2, 3, 4, 5, 7, 8, 9,
		10, 1000, 1000000, 1000000000, 60000000000, math.MaxInt32, 1 << 31,
		9007199254740991, 9007199254740992, 9007199254740993,
		4611686018427387903, 4611686018427387904, 4611686018427387905, math.MaxInt64 - 1, math.MaxInt64}
}

func durationInputs() [][]any {
	var inputs [][]any
	seen := map[[2]int64]bool{}
	add := func(d, m int64) {
		key := [2]int64{d, m}
		if !seen[key] {
			seen[key] = true
			inputs = append(inputs, []any{d, m})
		}
	}
	for d := int64(-16); d <= 16; d++ {
		for m := int64(-4); m <= 8; m++ {
			add(d, m)
		}
	}
	for _, d := range durationBoundaryValues() {
		for _, m := range durationQuanta() {
			add(d, m)
		}
	}
	// Build values mathematically before checking representability. Test input
	// generation must not accidentally wrap before it reaches the independent oracle.
	addBig := func(d *big.Int, m int64) {
		if d.IsInt64() {
			add(d.Int64(), m)
		}
	}
	for _, m := range durationQuanta() {
		if m <= 0 {
			continue
		}
		quantum := big.NewInt(m)
		for _, q := range []int64{0, 1, 2} {
			multiple := new(big.Int).Mul(quantum, big.NewInt(q))
			for _, center := range []*big.Int{multiple, new(big.Int).Add(multiple, big.NewInt(m/2))} {
				for _, delta := range []int64{-1, 0, 1} {
					d := new(big.Int).Add(center, big.NewInt(delta))
					addBig(d, m)
					addBig(new(big.Int).Neg(d), m)
				}
			}
		}
		// Check neighboring inputs around the final rounding threshold at each
		// signed bound, including cases whose next multiple would overflow.
		for _, limit := range []*big.Int{big.NewInt(math.MaxInt64), new(big.Int).Neg(big.NewInt(math.MinInt64))} {
			last := new(big.Int).Mul(new(big.Int).Quo(limit, quantum), quantum)
			threshold := new(big.Int).Add(last, big.NewInt(m/2))
			for _, center := range []*big.Int{last, threshold} {
				for _, delta := range []int64{-1, 0, 1} {
					d := new(big.Int).Add(center, big.NewInt(delta))
					addBig(d, m)
					addBig(new(big.Int).Neg(d), m)
				}
			}
		}
	}
	for _, pair := range [][2]int64{
		{1500000000, 1000000000}, {-1500000000, 1000000000},
		{1300000000, 1000000000}, {-1300000000, 1000000000},
		{8000000000000000000, 3000000000000000000}, {-8000000000000000000, 3000000000000000000},
		{9000000000000000000, 5000000000000000000}, {-9000000000000000000, 5000000000000000000},
		{6917529027641081855, 6917529027641081856},
	} {
		add(pair[0], pair[1])
	}
	rng := rand.New(rand.NewPCG(163, 211))
	for i := 0; i < 128; i++ {
		add(int64(rng.Uint64()), int64(rng.Uint64()))
	}
	return inputs
}

func durationQuantizerOracle(d, m int64) (any, error) {
	rounded := int64(time.Duration(d).Round(time.Duration(m)))
	truncated := int64(time.Duration(d).Truncate(time.Duration(m)))
	adjustment := new(big.Int).Sub(big.NewInt(rounded), big.NewInt(d))
	remainder := new(big.Int).Sub(big.NewInt(d), big.NewInt(truncated))
	if !adjustment.IsInt64() || !remainder.IsInt64() {
		return nil, fmt.Errorf("duration consumer difference outside int64 for (%d,%d)", d, m)
	}
	return map[string]any{"rounded": rounded, "truncated": truncated,
		"round_adjustment": adjustment.Int64(), "truncation_remainder": remainder.Int64()}, nil
}
