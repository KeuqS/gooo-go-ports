// Validate the new executable's identity before bootstrap publishes it.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type compilerPin struct {
	Revision string `json:"compiler_revision"`
	Version  string `json:"compiler_version"`
	Go       string `json:"go_version"`
}

type compilerBuild struct {
	Revision string `json:"compiler_source_sha"`
	Version  string `json:"version"`
	Go       string `json:"go_version"`
	Native   string `json:"native_go_required"`
	Status   string `json:"source_status"`
	Modified string `json:"vcs_modified"`
}

func validateBuild(raw []byte, pin compilerPin) error {
	var build compilerBuild
	if err := json.Unmarshal(raw, &build); err != nil {
		return fmt.Errorf("invalid compiler build identity: %w", err)
	}
	if pin.Revision == "" || pin.Version == "" || pin.Go == "" ||
		build.Revision != pin.Revision || build.Version != pin.Version ||
		build.Go != pin.Go || build.Native != pin.Go ||
		build.Status != "CLEAN_VCS" || build.Modified != "false" {
		return fmt.Errorf("compiler build identity differs from upstream.json; existing executable preserved")
	}
	return nil
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: bootstrap-check identity.json")
	}
	raw, err := os.ReadFile("upstream.json")
	if err != nil {
		return err
	}
	var pin compilerPin
	if err := json.Unmarshal(raw, &pin); err != nil {
		return err
	}
	raw, err = os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	return validateBuild(raw, pin)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap:", err)
		os.Exit(1)
	}
}
