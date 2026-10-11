package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Optional small real-compiler check; the full verifier covers all finite cases.
// Both environment paths must be explicit, and the output directory must be new.
func TestFixedNativeRepresentativeWorkspace(t *testing.T) {
	compiler := os.Getenv("GOOO_FIXED_TEST_COMPILER")
	out := os.Getenv("GOOO_FIXED_TEST_OUT")
	if compiler == "" && out == "" {
		t.Skip("set GOOO_FIXED_TEST_COMPILER and GOOO_FIXED_TEST_OUT for real native receipts")
	}
	if !filepath.IsAbs(compiler) || !filepath.IsAbs(out) {
		t.Fatal("explicit absolute compiler and output paths required")
	}
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var lock upstream
	if err := readJSON(filepath.Join(root, "upstream.json"), &lock); err != nil {
		t.Fatal(err)
	}
	if err := verifyFixedOracle(root, lock.ExternalSources); err != nil {
		t.Fatal(err)
	}
	catalog, err := freezeVerificationCatalog(filepath.Join(root, "verification-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	added, err := fixedCorpus()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, s := range added.Suites {
		if s.Package == fixedPackage {
			s.Cases = nil
			for _, n := range []int64{-2147483648, -2147483647, -65, -64, -63, -32, -1, 0, 1, 32, 63, 64, 65, 2147483583, 2147483584, 2147483585, 2147483647, -2147483649, 2147483648, 9007199254740993} {
				s.Cases = append(s.Cases, testCase{Inputs: []any{n}, Expected: fixedPortOracle(n, s.Activity == "Int26_6Ceil")})
			}
		} else {
			s.Cases = s.Cases[:16]
		}
		selected, err := catalog.workspaceForSuite(s)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(out, s.Activity)
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for name, raw := range selected.Sources {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0644); err != nil {
				t.Fatal(err)
			}
		}
		manifest, cases := filepath.Join(dir, "gooo.workspace.json"), filepath.Join(dir, "cases.json")
		if err := save(manifest, selected.Manifest); err != nil {
			t.Fatal(err)
		}
		if err := save(cases, caseDocument(s)); err != nil {
			t.Fatal(err)
		}
		m, err := digestFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		c, err := digestFile(cases)
		if err != nil {
			t.Fatal(err)
		}
		identity := expectedIdentity{CompilerRevision: lock.CompilerRevision, GoVersion: lock.GoVersion, ManifestDigest: "sha256:" + m, CasesDigest: "sha256:" + c, Sources: selected.Identities}
		receipt := filepath.Join(dir, "execution.json")
		raw, err := execute(ctx, compiler, receipt, "package", "execute", "--json", "--cases", cases, manifest)
		if err != nil {
			t.Fatal(err)
		}
		original, err := inspect(raw, s, false, identity)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := execute(ctx, compiler, filepath.Join(dir, "replay.json"), "package", "replay", "--json", "--receipt", receipt, "--cases", cases, manifest)
		if err != nil {
			t.Fatal(err)
		}
		checked, err := inspect(replay, s, true, identity)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := digestFile(receipt)
		if err != nil {
			t.Fatal(err)
		}
		if err := inspectReplayBinding(original, checked, "sha256:"+digest); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d original input/source/output checks and saved replay; generated %s", s.Activity, len(s.Cases), original.Generated)
		// These are actual receipt mutations, independent of synthetic fixtures.
		if s.Package == fixedPackage {
			bad := strings.Replace(string(raw), `"in_fixed_domain":true`, `"in_fixed_domain":false`, 1)
			if bad == string(raw) {
				t.Fatal("mutation did not find true flag")
			}
			if _, err := inspect([]byte(bad), s, false, identity); err == nil {
				t.Fatal("accepted real receipt flag mutation")
			}
		}
	}
}
