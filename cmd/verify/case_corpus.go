package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"unicode"
	"unicode/utf8"
)

const caseMappingPortBatchSize = 128
const caseMappingConsumerBatchSize = 64

type caseMappingObservations struct {
	Suites                                               []suite
	PortReferenceCases, PortExtensionCases               int
	InvalidCaseReferenceCases, InvalidCaseExtensionCases int // Subsets of the port counts.
	ConsumerReferenceCases, ConsumerExtensionCases       int
}

func caseMappingCorpus() (caseMappingObservations, error) {
	if unicode.Version != "17.0.0" || strconv.IntSize != 64 {
		return caseMappingObservations{}, fmt.Errorf("case mapping requires Unicode 17.0.0 and 64-bit Go int")
	}
	var result caseMappingObservations
	common := caseMappingBoundaryRunes()
	changed := caseMappingChangedRunes()
	operations := []string{"To", "ToUpper", "ToLower", "ToTitle"}
	for _, operation := range operations {
		var inputs [][]any
		modes := []int{0, 1, 2}
		if operation != "To" {
			modes = []int{caseMappingMode(operation)}
		}
		for _, mode := range modes {
			runes := caseMappingUniqueRunes(common, changed[mode])
			for _, r := range runes {
				if operation == "To" {
					inputs = append(inputs, []any{int64(mode), r})
				} else {
					inputs = append(inputs, []any{r})
				}
			}
		}
		if operation == "To" {
			for _, which := range caseMappingFiniteInvalidCases() {
				for _, r := range caseMappingFiniteInvalidCaseRunes() {
					inputs = append(inputs, []any{which, r})
					result.InvalidCaseReferenceCases++
				}
			}
		}
		s, err := caseMappingReferenceSuite("go/unicode", operation, inputs, func(which int, r rune) any {
			switch operation {
			case "To":
				return int64(unicode.To(which, r))
			case "ToUpper":
				return int64(unicode.ToUpper(r))
			case "ToLower":
				return int64(unicode.ToLower(r))
			default:
				return int64(unicode.ToTitle(r))
			}
		})
		if err != nil {
			return result, err
		}
		result.PortReferenceCases += len(s.Cases)
		for _, r := range simpleFoldExtendedInputs() {
			if operation == "To" {
				for _, which := range []int64{0, 1, 2} {
					s.Cases = append(s.Cases, testCase{Inputs: []any{which, r}, Expected: r})
					result.PortExtensionCases++
				}
				for _, which := range caseMappingFiniteInvalidCases() {
					s.Cases = append(s.Cases, testCase{Inputs: []any{which, r}, Expected: int64(unicode.ReplacementChar)})
					result.PortExtensionCases++
					result.InvalidCaseExtensionCases++
				}
			} else {
				s.Cases = append(s.Cases, testCase{Inputs: []any{r}, Expected: r})
				result.PortExtensionCases++
			}
		}
		s.Domain = "Original signed int32 rune; To additionally preserves the original 64-bit Go int case selector, including invalid cases. Finite Go references include every changing rune per mode, Latin-1, all CaseRanges boundaries/parities, invalid Unicode and deterministic samples. Outside-int32 is a separately counted identity extension; an invalid To case returns U+FFFD before this extension. No locale, SpecialCase, whole-string or multi-rune mapping."
		result.Suites = append(result.Suites, s)
	}
	consumerRunes := caseMappingUniqueRunes(common, changed[0], changed[1], changed[2])
	inputs := make([][]any, 0, len(consumerRunes))
	for _, r := range consumerRunes {
		inputs = append(inputs, []any{r})
	}
	consumer, err := caseMappingReferenceSuite("app/case-inspector", "Describe", inputs, func(_ int, r rune) any {
		return map[string]any{"in_rune_domain": true, "valid_scalar": utf8.ValidRune(r), "original": int64(r), "upper": int64(unicode.ToUpper(r)), "lower": int64(unicode.ToLower(r)), "title": int64(unicode.ToTitle(r))}
	})
	if err != nil {
		return result, err
	}
	result.ConsumerReferenceCases = len(consumer.Cases)
	for _, r := range simpleFoldExtendedInputs() {
		consumer.Cases = append(consumer.Cases, testCase{Inputs: []any{r}, Expected: caseMappingExtendedReport(r)})
		result.ConsumerExtensionCases++
	}
	consumer.Domain = "One original signed int32 rune; Go ToUpper/ToLower/ToTitle and utf8.ValidRune are independent references. Reports original and three simple one-rune mappings. Outside-int32 inputs preserve all four integer fields and return false domain/scalar flags, counted separately. No whole-string case conversion, title-word detection, full folding or locale handling."
	result.Suites = append(result.Suites, consumer)
	return result, nil
}

// Validate the complete original-domain roster before converting ANY case or
// rune, and before calling the independent Go reference. Extended integers are
// deliberately added separately and must never be narrowed to Go rune.
func caseMappingReferenceSuite(pkg, activity string, inputs [][]any, oracle func(int, rune) any) (suite, error) {
	arity := 1
	switch activity {
	case "To":
		arity = 2
	case "ToUpper", "ToLower", "ToTitle", "Describe":
	default:
		return suite{}, fmt.Errorf("unknown case mapping activity")
	}
	if strconv.IntSize != 64 || len(inputs) == 0 || oracle == nil {
		return suite{}, fmt.Errorf("case mapping requires a nonempty reference roster and 64-bit Go int")
	}
	seen := map[[2]int64]bool{}
	for _, input := range inputs {
		if len(input) != arity {
			return suite{}, fmt.Errorf("%s requires %d original inputs", activity, arity)
		}
		var key [2]int64
		for i, v := range input {
			n, ok := v.(int64)
			if !ok {
				return suite{}, fmt.Errorf("case mapping input%d must be an exact int64", i)
			}
			key[i] = n
			if i == arity-1 && (n < math.MinInt32 || n > math.MaxInt32) {
				return suite{}, fmt.Errorf("case mapping rune must remain inside original int32 domain")
			}
		}
		if seen[key] {
			return suite{}, fmt.Errorf("duplicate case mapping reference tuple")
		}
		seen[key] = true
	}
	s := suite{Package: pkg, Activity: activity}
	for _, input := range inputs {
		which := 0
		if arity == 2 {
			which = int(input[0].(int64))
		}
		r := rune(input[arity-1].(int64))
		s.Cases = append(s.Cases, testCase{Inputs: append([]any(nil), input...), Expected: oracle(which, r)})
	}
	return s, nil
}

func caseMappingMode(operation string) int {
	switch operation {
	case "ToUpper":
		return unicode.UpperCase
	case "ToLower":
		return unicode.LowerCase
	case "ToTitle":
		return unicode.TitleCase
	}
	panic("unknown case mapping operation")
}

// Coverage selection invokes the pinned Go functions, not the generated port
// or copied mapping formulas. This scan alone is not Gooo execution evidence.
func caseMappingChangedRunes() [3][]int64 {
	var changed [3][]int64
	for r := rune(0); r <= unicode.MaxRune; r++ {
		for mode, got := range []rune{unicode.ToUpper(r), unicode.ToLower(r), unicode.ToTitle(r)} {
			if got != r {
				changed[mode] = append(changed[mode], int64(r))
			}
		}
	}
	return changed
}
func caseMappingUniqueRunes(groups ...[]int64) []int64 {
	seen := map[int64]bool{}
	var values []int64
	for _, group := range groups {
		for _, r := range group {
			if !seen[r] {
				seen[r] = true
				values = append(values, r)
			}
		}
	}
	return values
}
func caseMappingBoundaryRunes() []int64 {
	var values []int64
	for r := int64(0); r <= 255; r++ {
		values = append(values, r)
	}
	for _, cr := range unicode.CaseRanges {
		for _, n := range []int64{int64(cr.Lo) - 1, int64(cr.Lo), int64(cr.Lo) + 1, int64(cr.Lo) + 2, int64(cr.Lo) + 3, int64(cr.Hi) - 3, int64(cr.Hi) - 2, int64(cr.Hi) - 1, int64(cr.Hi), int64(cr.Hi) + 1} {
			values = append(values, n)
		}
	}
	values = append(values, simpleFoldRuneBoundaries()...)
	values = append(values, 0xdf, 0x1e9e, 0x130, 0x131, 0x1c4, 0x1c5, 0x1c6, 0x1c7, 0x1c8, 0x1c9, 0x1ca, 0x1cb, 0x1cc, 0x1f1, 0x1f2, 0x1f3, 0x345, 0x3a3, 0x3c2, 0x212a, 0x13a, 0x10428, 0x1e922)
	rng := rand.New(rand.NewPCG(431, 433))
	for i := 0; i < 64; i++ {
		values = append(values, int64(int32(rng.Uint32())), int64(rng.Uint32N(0x110000)))
	}
	return caseMappingUniqueRunes(values)
}
func caseMappingFiniteInvalidCases() []int64 {
	return []int64{math.MinInt64, -9007199254740993, math.MinInt32, -2, -1, 3, 4, math.MaxInt32, math.MaxInt32 + 1, 4294967296, 9007199254740993, math.MaxInt64}
}
func caseMappingFiniteInvalidCaseRunes() []int64 {
	return []int64{math.MinInt32, -1, 0, 'A', 'a', 0x130, 0x345, 0xd800, 0x10ffff, 0x110000, math.MaxInt32}
}
func caseMappingExtendedReport(r int64) map[string]any {
	return map[string]any{"in_rune_domain": false, "valid_scalar": false, "original": r, "upper": r, "lower": r, "title": r}
}
