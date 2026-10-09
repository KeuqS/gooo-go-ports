#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
revision=366ee6ef8f9d2b9b57331dfd26b6f908c0289851
if [ ! -d .compiler/.git ]; then
  git init .compiler
  git -C .compiler remote add origin https://github.com/kimjooyoon/meta-ontology-go.git
  git -C .compiler fetch --depth=1 origin "$revision"
  git -C .compiler checkout --detach FETCH_HEAD
fi
test "$(git -C .compiler rev-parse HEAD)" = "$revision"
test -z "$(git -C .compiler status --porcelain)"
test "$(go env GOVERSION)" = go1.27.2
(cd .compiler && go build -trimpath -o ../.gooo ./cmd/gooo)
./.gooo version --build --json
