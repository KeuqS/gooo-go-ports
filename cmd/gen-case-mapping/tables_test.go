package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

func pinnedTableSource(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "src/unicode/tables.go"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func pinnedTables(t *testing.T) caseTables {
	t.Helper()
	tables, err := parseTables(pinnedTableSource(t), unicode.CaseRanges)
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

func TestPinnedCaseTablesAreComplete(t *testing.T) {
	tables := pinnedTables(t)
	if len(tables.ranges) != 336 {
		t.Fatal("pinned range count changed", len(tables.ranges))
	}
	alternating, oddOrigin, titleDifferent := 0, 0, 0
	for _, r := range tables.ranges {
		if r.delta[0] == unicode.UpperLower {
			alternating++
			if r.lo%2 == 1 {
				oddOrigin++
			}
		}
		if r.delta[0] != r.delta[2] {
			titleDifferent++
		}
	}
	if alternating != 58 || oddOrigin != 22 || titleDifferent != 14 {
		t.Fatal("pinned alternating/title data changed", alternating, oddOrigin, titleDifferent)
	}
	// Manual upstream rows detect accidental substitution of fold or locale
	// data. In particular title-case digraphs have three distinct outputs.
	for _, want := range []caseRange{
		{0x41, 0x5a, [3]int64{0, 32, 0}},
		{0x130, 0x130, [3]int64{0, -199, 0}},
		{0x131, 0x131, [3]int64{-232, 0, -232}},
		{0x139, 0x148, [3]int64{unicode.UpperLower, unicode.UpperLower, unicode.UpperLower}},
		{0x1c5, 0x1c5, [3]int64{-1, 1, 0}},
		{0x10428, 0x1044f, [3]int64{-40, 0, -40}},
	} {
		found := false
		for _, r := range tables.ranges {
			if r == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing upstream row %#v", want)
		}
	}
}

func TestTableParserRejectsChangedASTOrCompiledData(t *testing.T) {
	source := string(pinnedTableSource(t))
	for _, test := range []struct{ name, before, after string }{
		{"missing table", "var _CaseRanges =", "var hiddenCaseRanges ="},
		{"alias", "var CaseRanges = _CaseRanges", "var CaseRanges = otherRanges"},
		{"element type", "var _CaseRanges = []CaseRange{", "var _CaseRanges = []OtherRange{"},
		{"array instead of slice", "var _CaseRanges = []CaseRange{", "var _CaseRanges = [336]CaseRange{"},
		{"computed delta", "{0x0041, 0x005A, d{0, 32, 0}}", "{0x0041, 0x005A, d{0, compute(), 0}}"},
		{"keyed range", "{0x0041, 0x005A, d{0, 32, 0}}", "{Lo: 0x0041, Hi: 0x005A, Delta: d{0, 32, 0}}"},
		{"keyed delta", "{0x0041, 0x005A, d{0, 32, 0}}", "{0x0041, 0x005A, d{0:0, 1:32, 2:0}}"},
		{"delta type", "{0x0041, 0x005A, d{0, 32, 0}}", "{0x0041, 0x005A, other{0, 32, 0}}"},
		{"wrong version", `const Version = "17.0.0"`, `const Version = "16.0.0"`},
		{"wrong package", "package unicode", "package other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(source, test.before) {
				t.Fatal("mutation does not match")
			}
			if _, err := parseTables([]byte(strings.Replace(source, test.before, test.after, 1)), unicode.CaseRanges); err == nil {
				t.Fatal("changed source accepted")
			}
		})
	}
	if _, err := parseTables([]byte(source+"\nvar _CaseRanges = []CaseRange{}\n"), unicode.CaseRanges); err == nil {
		t.Fatal("duplicate table accepted")
	}
	changed := append([]unicode.CaseRange(nil), unicode.CaseRanges...)
	changed[0].Delta[unicode.TitleCase]++
	if _, err := parseTables([]byte(source), changed); err == nil {
		t.Fatal("compiled/source mismatch accepted")
	}
}

func TestTableValidationRejectsUnsafeMappings(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*caseTables)
	}{
		{"empty", func(t *caseTables) { t.ranges = nil }},
		{"reversed", func(t *caseTables) { t.ranges[0].hi = 0 }},
		{"negative", func(t *caseTables) { t.ranges[0].lo = -1 }},
		{"above Unicode", func(t *caseTables) { t.ranges[len(t.ranges)-1].hi = 0x110000 }},
		{"overlap", func(t *caseTables) { t.ranges[1].lo = t.ranges[0].hi }},
		{"partial alternating", func(t *caseTables) { t.ranges[0].delta = [3]int64{unicode.UpperLower, 0, 0} }},
		{"odd alternating length", func(t *caseTables) { t.ranges[8].hi-- }},
		{"negative output", func(t *caseTables) { t.ranges[0].delta[0] = -1000 }},
		{"overflow output", func(t *caseTables) { t.ranges[0].delta[0] = unicode.MaxRune }},
	} {
		t.Run(test.name, func(t *testing.T) {
			tables := pinnedTables(t)
			test.change(&tables)
			if err := validateTables(tables); err == nil {
				t.Fatal("invalid table accepted")
			}
		})
	}
}
