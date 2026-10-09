#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
compiler="$root/.gooo"
out=

usage() {
    printf 'Usage: %s --out NEW_DIRECTORY [--compiler PATH]\n' "$0"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --out)
            [ "$#" -ge 2 ] || { usage >&2; exit 2; }
            out=$2
            shift 2
            ;;
        --compiler)
            [ "$#" -ge 2 ] || { usage >&2; exit 2; }
            compiler=$2
            shift 2
            ;;
        --help|-h) usage; exit 0 ;;
        *) usage >&2; exit 2 ;;
    esac
done

[ -n "$out" ] || { usage >&2; exit 2; }
[ -x "$compiler" ] || { printf 'Compiler is not executable: %s\n' "$compiler" >&2; exit 2; }
[ ! -e "$out" ] || { printf 'Output directory must be new: %s\n' "$out" >&2; exit 2; }
mkdir -p -- "$out"
out=$(CDPATH= cd -- "$out" && pwd)
"$compiler" version --build --json > "$out/compiler-build.json"
printf 'probe\tcheck_exit\tbody_codegen_exit\tdiagnostic\n' > "$out/summary.tsv"

for name in loop byte-index bitwise; do
    case "$name" in
        loop) expected='unsupported activity statement *ast.ForStmt' ;;
        byte-index) expected='unsupported activity expression *ast.IndexExpr' ;;
        bitwise) expected='unsupported binary operator &' ;;
    esac

    cp -- "$root/probes/$name.gooo" "$out/$name.gooo"
    check_status=0
    "$compiler" check "$out/$name.gooo" > "$out/$name.check.stdout.txt" \
        2> "$out/$name.check.stderr.txt" || check_status=$?
    generation_status=0
    "$compiler" body-codegen --json --activity Probe "$out/$name.gooo" \
        > "$out/$name.body-codegen.stdout.json" \
        2> "$out/$name.body-codegen.stderr.txt" || generation_status=$?
    printf '%s\t%s\t%s\t%s\n' "$name" "$check_status" "$generation_status" "$expected" \
        >> "$out/summary.tsv"

    [ "$check_status" -eq 0 ] || {
        printf 'Unexpected declaration-check failure for %s; inspect %s\n' "$name" "$out" >&2
        exit 1
    }
    [ "$generation_status" -ne 0 ] || {
        printf 'Body generation accepted %s; update this boundary probe before claiming support\n' "$name" >&2
        exit 1
    }
    grep -F -- "\"error\":\"$expected\"" "$out/$name.body-codegen.stdout.json" > /dev/null || {
        printf 'Unexpected body-generation diagnostic for %s; inspect %s\n' "$name" "$out" >&2
        exit 1
    }
done

printf 'Recorded 3 declaration-valid, unsupported body probes in %s\n' "$out"
