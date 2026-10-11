package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"unicode"
)

type caseRange struct {
	lo, hi int64
	delta  [3]int64
}
type caseTables struct{ ranges []caseRange }

// Read the CaseRanges literal through Go's AST and compare all Lo/Hi/Delta
// values with the exported data. Neither path calls To or its wrappers; those
// functions remain independent test oracles, not generator inputs.
func parseTables(source []byte, exported []unicode.CaseRange) (caseTables, error) {
	var result caseTables
	f, err := parser.ParseFile(token.NewFileSet(), "tables.go", source, 0)
	if err != nil {
		return result, fmt.Errorf("parse pinned Unicode tables: %w", err)
	}
	if f.Name.Name != "unicode" {
		return result, fmt.Errorf("table source must be package unicode")
	}
	values := map[string]ast.Expr{}
	wanted := map[string]bool{"Version": true, "CaseRanges": true, "_CaseRanges": true}
	for _, declaration := range f.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR && gen.Tok != token.CONST {
			continue
		}
		for _, specification := range gen.Specs {
			spec := specification.(*ast.ValueSpec)
			for _, name := range spec.Names {
				if !wanted[name.Name] {
					continue
				}
				if len(spec.Names) != 1 || len(spec.Values) != 1 || values[name.Name] != nil {
					return result, fmt.Errorf("ambiguous table declaration %s", name.Name)
				}
				values[name.Name] = spec.Values[0]
			}
		}
	}
	for name := range wanted {
		if values[name] == nil {
			return result, fmt.Errorf("missing table declaration %s", name)
		}
	}
	version, ok := values["Version"].(*ast.BasicLit)
	if !ok || version.Kind != token.STRING || version.Value != strconv.Quote(pinnedUnicode) {
		return result, fmt.Errorf("table Unicode version differs")
	}
	alias, ok := values["CaseRanges"].(*ast.Ident)
	if !ok || alias.Name != "_CaseRanges" {
		return result, fmt.Errorf("CaseRanges must alias the pinned literal table")
	}
	cases, err := tableLiteral(values["_CaseRanges"], "CaseRange", -1)
	if err != nil {
		return result, fmt.Errorf("CaseRanges: %w", err)
	}
	var sourceRanges []caseRange
	for _, item := range cases.Elts {
		row, ok := item.(*ast.CompositeLit)
		if !ok || row.Type != nil || len(row.Elts) != 3 {
			return result, fmt.Errorf("CaseRanges requires positional Lo/Hi/Delta literals")
		}
		lo, e1 := literalInteger(row.Elts[0])
		hi, e2 := literalInteger(row.Elts[1])
		delta, ok := row.Elts[2].(*ast.CompositeLit)
		if e1 != nil || e2 != nil || !ok || len(delta.Elts) != 3 {
			return result, fmt.Errorf("invalid CaseRanges literal")
		}
		name, ok := delta.Type.(*ast.Ident)
		if !ok || name.Name != "d" {
			return result, fmt.Errorf("CaseRanges Delta must be a d literal")
		}
		r := caseRange{lo: lo, hi: hi}
		for i, item := range delta.Elts {
			n, err := literalInteger(item)
			if err != nil {
				return result, fmt.Errorf("invalid CaseRanges delta: %w", err)
			}
			r.delta[i] = n
		}
		sourceRanges = append(sourceRanges, r)
	}
	for _, r := range exported {
		result.ranges = append(result.ranges, caseRange{int64(r.Lo), int64(r.Hi), [3]int64{int64(r.Delta[0]), int64(r.Delta[1]), int64(r.Delta[2])}})
	}
	if !reflect.DeepEqual(sourceRanges, result.ranges) {
		return result, fmt.Errorf("exported CaseRanges differ from the pinned source literal")
	}
	return result, validateTables(result)
}

func tableLiteral(value ast.Expr, element string, length int64) (*ast.CompositeLit, error) {
	literal, ok := value.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("expected table composite literal")
	}
	array, ok := literal.Type.(*ast.ArrayType)
	if !ok {
		return nil, fmt.Errorf("expected array or slice literal")
	}
	name, ok := array.Elt.(*ast.Ident)
	if !ok || name.Name != element {
		return nil, fmt.Errorf("unexpected table element type")
	}
	if length < 0 {
		if array.Len != nil {
			return nil, fmt.Errorf("expected slice literal")
		}
	} else {
		n, err := literalInteger(array.Len)
		if err != nil || n != length || int64(len(literal.Elts)) != length {
			return nil, fmt.Errorf("expected complete %d-element array literal", length)
		}
	}
	return literal, nil
}

func literalInteger(expression ast.Expr) (int64, error) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind == token.INT {
			return strconv.ParseInt(value.Value, 0, 64)
		}
	case *ast.Ident:
		switch value.Name {
		case "MaxASCII":
			return unicode.MaxASCII, nil
		case "UpperLower":
			return unicode.UpperLower, nil
		}
	case *ast.UnaryExpr:
		if value.Op == token.SUB {
			n, err := literalInteger(value.X)
			if err == nil && n >= 0 {
				return -n, nil
			}
		}
	case *ast.BinaryExpr:
		// The pinned array length is MaxASCII + 1. Do not evaluate arbitrary
		// source expressions or silently accept a changed table format.
		left, ok := value.X.(*ast.Ident)
		right, lit := value.Y.(*ast.BasicLit)
		if value.Op == token.ADD && ok && left.Name == "MaxASCII" && lit && right.Kind == token.INT && right.Value == "1" {
			return unicode.MaxASCII + 1, nil
		}
	}
	return 0, fmt.Errorf("unsupported table integer expression %T", expression)
}

func validateTables(t caseTables) error {
	if len(t.ranges) == 0 {
		return fmt.Errorf("case ranges are incomplete")
	}
	for i, r := range t.ranges {
		if r.lo < 0 || r.lo > r.hi || r.hi > unicode.MaxRune || i > 0 && t.ranges[i-1].hi >= r.lo {
			return fmt.Errorf("case ranges are invalid, unsorted or overlapping at %d", i)
		}
		alternating := r.delta == [3]int64{unicode.UpperLower, unicode.UpperLower, unicode.UpperLower}
		if alternating {
			if (r.hi-r.lo)%2 != 1 {
				return fmt.Errorf("UpperLower range must contain complete pairs")
			}
			continue
		}
		for _, delta := range r.delta {
			if delta < -unicode.MaxRune || delta > unicode.MaxRune || r.lo+delta < 0 || r.hi+delta > unicode.MaxRune {
				return fmt.Errorf("invalid case delta at %d", i)
			}
		}
	}
	return nil
}
