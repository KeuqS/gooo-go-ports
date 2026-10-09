# Gooo porting contract

- Implement ported behavior in `ports/**/*.gooo`; Go owns oracle execution and evidence checking.
- Preserve upstream copyright, license, immutable source revision and source-file hashes.
- Document Go-to-Gooo type/domain and API differences explicitly. Do not hide unsupported semantics behind a Go wrapper.
- Keep compiler and Go toolchain identities pinned in upstream.json and bootstrap/CI.
- Use independent Go reference outputs, boundary cases, deterministic corpora and saved replay.
- Match receipt inputs, outputs, entry identities, source bindings and producer identities; a PASS label alone is insufficient.
- Record unsupported syntax as executable probes. Never equate declaration checking with generated native execution.
- Required checks: gofmt, go vet ./..., go test -race ./..., native differential verifier, and boundary probes.
- Keep observations finite and reproducible. Do not claim full-library compatibility from function-level observations.
