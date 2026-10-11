// This local oracle-provenance helper is not part of upstream x/image.
// fixed.go is byte-preserved; expose its compiled-in digest for the verifier.
package fixed

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
)

//go:embed fixed.go
var pinnedSource []byte

func PinnedSourceSHA256() string { return fmt.Sprintf("%x", sha256.Sum256(pinnedSource)) }
