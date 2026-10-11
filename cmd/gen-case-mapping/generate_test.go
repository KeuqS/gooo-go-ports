package main

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

func pinnedIdentity(t *testing.T) upstream {
	t.Helper()
	raw, err := os.ReadFile("../../upstream.json")
	if err != nil {
		t.Fatal(err)
	}
	var pin upstream
	if err := json.Unmarshal(raw, &pin); err != nil {
		t.Fatal(err)
	}
	if err := verifyIdentity(pin, runtime.Version(), unicode.Version, runtime.GOROOT()); err != nil {
		t.Fatal(err)
	}
	return pin
}

func generatedFixture(t *testing.T) []byte {
	t.Helper()
	source, err := generate(pinnedIdentity(t), pinnedTables(t))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestGeneratedSourceIsDeterministicCurrentAndBounded(t *testing.T) {
	first, second := generatedFixture(t), generatedFixture(t)
	if !bytes.Equal(first, second) || len(first) > 128<<10 {
		t.Fatal("nondeterministic or oversized source")
	}
	current, err := os.ReadFile(filepath.Join("../..", targetSource))
	if err != nil || !bytes.Equal(first, current) {
		t.Fatal("generated source is stale", err)
	}
	for _, want := range []string{"Copyright 2009 The Go Authors", "third_party/GO-LICENSE", pinnedGoRevision,
		`id "gooo://go/unicode/To"`, `return To(0, input)`, `return To(1, input)`, `return To(2, input)`} {
		if !bytes.Contains(first, []byte(want)) {
			t.Fatal("missing provenance or shared mapping", want)
		}
	}
	// This checks syntax and canonical body framing only. Actual compiler
	// execution/replay and the unchanged-generated-Go oracle checks live in
	// cmd/verify; no second case-mapping interpreter is used as native proof.
	bodies := regexp.MustCompile(" computes `([^`]+)`").FindAllSubmatch(first, -1)
	if len(bodies) != 4 {
		t.Fatal("wrong activity body count", len(bodies))
	}
	for _, body := range bodies {
		if !bytes.Equal(body[1], bytes.TrimSpace(body[1])) {
			t.Fatal("noncanonical body")
		}
		if _, err := parser.ParseFile(token.NewFileSet(), "body.go", "package p\nfunc F(input,input0,input1 int64) int64 {\n"+string(body[1])+"\n}", 0); err != nil {
			t.Fatal(err)
		}
	}
	text := string(first)
	if strings.Index(text, "return 65533") > strings.Index(text, "input1 < -2147483648") {
		t.Fatal("invalid case lost precedence")
	}
}

func TestGeneratedLeavesPreserveRelativeParityAndTitleDeltas(t *testing.T) {
	var alternating strings.Builder
	writeCases(&alternating, []caseRange{{0x139, 0x148, [3]int64{unicode.UpperLower, unicode.UpperLower, unicode.UpperLower}}}, 0)
	want := "if input1 < 313 || input1 > 328 { return input1 }\nif input0 == 1 { return input1 + 1 - (input1 - 313) % 2 }\nreturn input1 - (input1 - 313) % 2"
	if alternating.String() != want {
		t.Fatal("odd-origin pair generation changed", alternating.String())
	}
	var title strings.Builder
	writeCases(&title, []caseRange{{0x1c5, 0x1c5, [3]int64{-1, 1, 0}}}, 0)
	want = "if input1 < 453 || input1 > 453 { return input1 }\nif input0 == 0 { return input1 - 1 }\nif input0 == 1 { return input1 + 1 }\nreturn input1"
	if title.String() != want {
		t.Fatal("title digraph was merged with uppercase", title.String())
	}
}

func TestGenerationRejectsOversizedSource(t *testing.T) {
	// Valid disjoint mappings can still exceed the native composition bound.
	// Reject that growth before writing a source that only declaration-checks.
	tables := caseTables{}
	for i := int64(0); i < 2048; i++ {
		tables.ranges = append(tables.ranges, caseRange{2 * i, 2 * i, [3]int64{0, 1, 0}})
	}
	if _, err := generate(pinnedIdentity(t), tables); err == nil || !strings.Contains(err.Error(), "128 KiB") {
		t.Fatal("oversized valid source was not rejected", err)
	}
}

func TestCLIWriteAndCheckDoNotRepairStaleSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ports/unicode"), 0755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../upstream.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "upstream.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"--root", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--root", root, "--check"}, &output); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, targetSource)
	stale := []byte("stale\n")
	if err := os.WriteFile(path, stale, 0644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--root", root, "--check"}, &output); err == nil {
		t.Fatal("stale source accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, stale) {
		t.Fatal("--check wrote output", err)
	}
}
