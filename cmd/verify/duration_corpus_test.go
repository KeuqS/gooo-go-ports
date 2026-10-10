package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"
)

func TestDurationCoverageAndExactBatchInputs(t *testing.T) {
	added, err := durationCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Suites) != 3 || added.PortReferenceCases != 3578 || added.ConsumerReferenceCases != 1789 {
		t.Fatalf("unexpected duration coverage: %+v", added)
	}
	for index, s := range added.Suites {
		seen := map[[2]int64]bool{}
		for i, c := range s.Cases {
			key := [2]int64{c.Inputs[0].(int64), c.Inputs[1].(int64)}
			if seen[key] {
				t.Fatalf("duplicate %s input %v", s.Activity, key)
			}
			seen[key] = true
			if !reflect.DeepEqual(c.Inputs, added.Suites[0].Cases[i].Inputs) {
				t.Fatal("duration suites changed the original input roster")
			}
		}
		for d := int64(-16); d <= 16; d++ {
			for m := int64(-4); m <= 8; m++ {
				if !seen[[2]int64{d, m}] {
					t.Fatalf("missing small-grid pair (%d,%d)", d, m)
				}
			}
		}
		for _, d := range durationBoundaryValues() {
			for _, m := range durationQuanta() {
				if !seen[[2]int64{d, m}] {
					t.Fatalf("missing exact boundary (%d,%d)", d, m)
				}
			}
		}
		batchSize := durationPortBatchSize
		if index == 2 {
			batchSize = durationQuantizerBatchSize
		}
		maxBytes, batches := 0, 0
		for start := 0; start < len(s.Cases); start += batchSize {
			batch := s
			batch.Cases = s.Cases[start:min(start+batchSize, len(s.Cases))]
			raw, err := json.MarshalIndent(caseDocument(batch), "", "  ")
			if err != nil || len(batch.Cases) > 128 || len(raw)+1 > 32<<10 {
				t.Fatalf("%s batch exceeds compiler budget: %d cases, %d bytes, %v", s.Activity, len(batch.Cases), len(raw)+1, err)
			}
			maxBytes = max(maxBytes, len(raw)+1)
			batches++
			var doc struct {
				Cases []struct {
					Inputs map[string]json.RawMessage `json:"inputs"`
				} `json:"cases"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			for i, row := range doc.Cases {
				if len(row.Inputs) != 2 {
					t.Fatal("serialized duration input arity changed")
				}
				for j, n := range batch.Cases[i].Inputs {
					key := fmt.Sprintf("%s:%s.input%d", s.Package, s.Activity, j)
					if !equalValue(row.Inputs[key], n) {
						t.Fatalf("input narrowed or rounded before receipt: %s %v", key, n)
					}
				}
			}
		}
		t.Logf("%s: %d cases, %d batches, maximum saved batch %d bytes", s.Activity, len(s.Cases), batches, maxBytes)
	}
}

func TestDurationRosterValidatedBeforeOracle(t *testing.T) {
	valid := []any{int64(math.MinInt64), int64(math.MaxInt64)}
	invalid := [][]any{nil, {}, valid[:1], {int64(0), int64(0), int64(0)}, valid}
	for _, v := range []any{int(1), float64(1), "1", json.Number("1"), uint64(math.MaxUint64), nil} {
		invalid = append(invalid, []any{v, int64(0)}, []any{int64(0), v})
	}
	for _, bad := range invalid {
		calls := 0
		_, err := durationSuite("test", "duration", [][]any{valid, bad}, func(int64, int64) (any, error) { calls++; return nil, nil })
		if err == nil || calls != 0 {
			t.Fatalf("invalid roster reached oracle: %v calls=%d err=%v", bad, calls, err)
		}
	}
	if _, err := durationSuite("test", "duration", nil, func(int64, int64) (any, error) { t.Fatal("empty roster called oracle"); return nil, nil }); err == nil {
		t.Fatal("empty corpus accepted")
	}
	inputs := [][]any{{int64(math.MinInt64), int64(math.MinInt64)}, {int64(math.MaxInt64), int64(-1)}, {int64(9007199254740993), int64(0)}, {int64(-9007199254740993), int64(9007199254740993)}}
	var observed [][]any
	s, err := durationSuite("test", "duration", inputs, func(d, m int64) (any, error) { observed = append(observed, []any{d, m}); return nil, nil })
	if err != nil || !reflect.DeepEqual(inputs, observed) {
		t.Fatalf("full signed domain changed: %v %v", observed, err)
	}
	inputs[0][0] = int64(0)
	if s.Cases[0].Inputs[0] != int64(math.MinInt64) {
		t.Fatal("saved input aliases caller storage")
	}
}

func TestDurationNamedResultsAndExactConsumerDeltas(t *testing.T) {
	added, err := durationCorpus()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ d, m, rounded, truncated int64 }{
		{1500000000, 1000000000, 2000000000, 1000000000},
		{-1500000000, 1000000000, -2000000000, -1000000000},
		{2, 5, 0, 0}, {3, 5, 5, 0}, {-2, 5, 0, 0}, {-3, 5, -5, 0},
		{math.MaxInt64, 2, math.MaxInt64, math.MaxInt64 - 1},
		{math.MaxInt64 - 1, 4, math.MaxInt64, math.MaxInt64 - 3},
		{math.MinInt64, 3, math.MinInt64, math.MinInt64 + 2},
		{math.MinInt64 + 1, 3, math.MinInt64 + 2, math.MinInt64 + 2},
		{math.MinInt64 + 1, 2, math.MinInt64, math.MinInt64 + 2},
		{math.MaxInt64 / 2, math.MaxInt64, 0, 0},
		{math.MaxInt64/2 + 1, math.MaxInt64, math.MaxInt64, 0},
		{math.MinInt64, math.MaxInt64, -math.MaxInt64, -math.MaxInt64},
		{9007199254740992, 3, 9007199254740993, 9007199254740990},
		{9007199254740993, 3, 9007199254740993, 9007199254740993},
	} {
		checkConsumerCase(t, added.Suites[0], []any{c.d, c.m}, c.rounded)
		checkConsumerCase(t, added.Suites[1], []any{c.d, c.m}, c.truncated)
	}
	for _, c := range added.Suites[2].Cases {
		d, m := c.Inputs[0].(int64), c.Inputs[1].(int64)
		v := c.Expected.(map[string]any)
		for _, pair := range [][2]string{{"rounded", "round_adjustment"}, {"truncated", "truncation_remainder"}} {
			difference := new(big.Int).Sub(big.NewInt(v[pair[0]].(int64)), big.NewInt(d))
			if pair[0] == "truncated" {
				difference.Neg(difference)
			}
			if !difference.IsInt64() || difference.Int64() != v[pair[1]].(int64) {
				t.Fatalf("consumer delta is not exact: %v", c)
			}
		}
		if m <= 0 && (v["rounded"] != d || v["truncated"] != d || v["round_adjustment"] != int64(0) || v["truncation_remainder"] != int64(0)) {
			t.Fatalf("nonpositive quantum changed duration: %v", c)
		}
	}
}

func TestDurationPreservesExisting8726Cases(t *testing.T) {
	legacy := func() []suite {
		base, err := corpus()
		if err != nil {
			t.Fatal(err)
		}
		unicode, err := unicodeInspectorCorpus(base)
		if err != nil {
			t.Fatal(err)
		}
		colors, err := colorCorpus()
		if err != nil {
			t.Fatal(err)
		}
		rotations, err := rotationCorpus()
		if err != nil {
			t.Fatal(err)
		}
		return append(append(append(base, unicode.Suites...), colors.Suites...), rotations.Suites...)
	}
	before := legacy()
	total := 0
	for _, s := range before {
		total += len(s.Cases)
	}
	if total != 8726 {
		t.Fatalf("existing case count changed: %d", total)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(mustJSON(t, before)))
	// Frozen from the unchanged de030c5 corpus before adding duration suites.
	const existing = "fce90fd71283600e6432970cd3de038b2b9d90a9d9099748e8effaa8980094c8"
	if hash != existing {
		t.Fatalf("existing 8726-case corpus changed: %s", hash)
	}
	if _, err := durationCorpus(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, legacy()) {
		t.Fatal("duration construction mutated old corpus")
	}
}
