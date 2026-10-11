package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

func inspectCaseMappingGenerated(raw []byte, expectedGenerated, operation string) (string, string, error) {
	cases, err := caseMappingValidCases(operation)
	if err != nil {
		return "", "", err
	}
	var receipt struct {
		Result struct {
			Composition struct {
				Stage, Source string
				Generated     string `json:"generated_sha256"`
				Plan          struct {
					Schema     string
					Activities []struct {
						ID         string
						Function   string `json:"go_function"`
						Input      string `json:"input_entity_id"`
						Output     string `json:"output_entity_id"`
						InputType  string `json:"input_type"`
						OutputType string `json:"output_type"`
						InputFrom  int    `json:"input_from"`
						Assembling bool
						Inputs     []struct {
							Port, Type string
							Entity     string `json:"entity_id"`
							From       int
						}
					}
					Edges        []json.RawMessage
					Preparations []json.RawMessage
				} `json:"plan"`
			} `json:"composition"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return "", "", err
	}
	c := receipt.Result.Composition
	if !validDigest(expectedGenerated) || c.Stage != "COMPLETE" || c.Generated != expectedGenerated || c.Source == "" || contentDigest([]byte(c.Source)) != expectedGenerated {
		return "", "", fmt.Errorf("case mapping checked generated-source binding differs")
	}
	if c.Plan.Schema != "gooo/body-composition-plan/v1" || len(c.Plan.Activities) != 1 || len(c.Plan.Edges) != 0 || len(c.Plan.Preparations) != 0 {
		return "", "", fmt.Errorf("case mapping requires one independent planned activity")
	}
	a := c.Plan.Activities[0]
	if a.ID != "gooo://go/unicode/"+operation || a.Input != integerEntity || a.Output != integerEntity || a.InputType != "Integer" || a.OutputType != "Integer" || a.InputFrom != -1 || a.Assembling || !caseMappingFunctionSymbol(a.Function) {
		return "", "", fmt.Errorf("case mapping planned activity identity or ABI differs")
	}
	arity := 1
	if operation == "To" {
		arity = 2
		if len(a.Inputs) != 2 {
			return "", "", fmt.Errorf("To requires two planned inputs")
		}
		for i, input := range a.Inputs {
			if input.Port != fmt.Sprintf("input%d", i) || input.Type != "Integer" || input.Entity != integerEntity || input.From != -1 {
				return "", "", fmt.Errorf("To planned input identity or order differs")
			}
		}
	} else if len(a.Inputs) != 0 {
		return "", "", fmt.Errorf("unary case mapping requires the canonical single input")
	}
	f, err := parser.ParseFile(token.NewFileSet(), "generated.go", c.Source, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return "", "", err
	}
	if f.Name.Name != "main" || len(f.Imports) != 0 {
		return "", "", fmt.Errorf("case mapping generated source must be package main without imports")
	}
	for _, group := range f.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:") || strings.HasPrefix(comment.Text, "//line ") || strings.HasPrefix(comment.Text, "/*line ") {
				return "", "", fmt.Errorf("case mapping generated source contains compiler directives")
			}
		}
	}
	functions := map[string]*ast.FuncDecl{}
	declarations := map[string]bool{}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			inputs := arity
			if d.Name.Name != a.Function {
				// The pinned compiler lowers a unary wrapper's sole To call to
				// this named two-input helper, which is preserved unchanged.
				if operation == "To" || d.Name.Name != a.Function+"Call0" {
					return "", "", fmt.Errorf("case mapping generated helper roster differs")
				}
				inputs = 2
			}
			if declarations[d.Name.Name] || d.Recv != nil || d.Body == nil || d.Type.TypeParams != nil || !caseMappingInt64Inputs(d.Type.Params, inputs) || !ycbcrRecordResult(d.Type.Results, "int64") {
				return "", "", fmt.Errorf("case mapping generated function signature differs")
			}
			declarations[d.Name.Name], functions[d.Name.Name] = true, d
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				return "", "", fmt.Errorf("case mapping generated source contains a non-record declaration")
			}
			for _, spec := range d.Specs {
				t, ok := spec.(*ast.TypeSpec)
				if !ok || t.Assign.IsValid() || t.TypeParams != nil || !ycbcrExportedSymbol(t.Name.Name) || declarations[t.Name.Name] || strings.HasPrefix(t.Name.Name, "Test") {
					return "", "", fmt.Errorf("case mapping generated record declaration differs")
				}
				s, ok := t.Type.(*ast.StructType)
				if !ok || !ycbcrInertRecord(s) {
					return "", "", fmt.Errorf("case mapping generated type is not an inert primitive record")
				}
				declarations[t.Name.Name] = true
			}
		default:
			return "", "", fmt.Errorf("case mapping generated declaration differs")
		}
	}
	wantFunctions := 1
	if operation != "To" {
		wantFunctions = 2
		if !caseMappingWrapperBody(functions[a.Function], a.Function+"Call0", cases[0]) {
			return "", "", fmt.Errorf("case mapping wrapper must call the shared helper with its fixed case and original input")
		}
	}
	if len(functions) != wantFunctions || functions[a.Function] == nil {
		return "", "", fmt.Errorf("case mapping generated function roster differs")
	}
	for name, function := range functions {
		bad := false
		ast.Inspect(function.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				id, direct := call.Fun.(*ast.Ident)
				// No oracle/harness references, arbitrary calls or recursion.
				if !direct || (id.Name != "int64" && id.Name != "int32" && !(operation != "To" && name == a.Function && id.Name == a.Function+"Call0")) {
					bad = true
				}
			}
			return !bad
		})
		if bad {
			return "", "", fmt.Errorf("case mapping generated source calls an unexpected function")
		}
	}
	return c.Source, a.Function, nil
}

func caseMappingFunctionSymbol(name string) bool {
	return ycbcrExportedSymbol(name) && !strings.HasPrefix(name, "Test") && !strings.HasPrefix(name, "Benchmark") && !strings.HasPrefix(name, "Fuzz") && !strings.HasPrefix(name, "Example")
}

func caseMappingInt64Inputs(fields *ast.FieldList, arity int) bool {
	if fields == nil || len(fields.List) != arity {
		return false
	}
	seen := map[string]bool{}
	for _, field := range fields.List {
		t, ok := field.Type.(*ast.Ident)
		if !ok || t.Name != "int64" || len(field.Names) != 1 || !ycbcrSymbol(field.Names[0].Name) || seen[field.Names[0].Name] {
			return false
		}
		seen[field.Names[0].Name] = true
	}
	return true
}

func caseMappingWrapperBody(f *ast.FuncDecl, helper string, c int64) bool {
	if f == nil || len(f.Body.List) != 1 {
		return false
	}
	ret, ok := f.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return false
	}
	name, nOK := call.Fun.(*ast.Ident)
	value, vOK := call.Args[0].(*ast.BasicLit)
	input, iOK := call.Args[1].(*ast.Ident)
	return nOK && vOK && iOK && name.Name == helper && value.Kind == token.INT && value.Value == strconv.FormatInt(c, 10) && input.Name == f.Type.Params.List[0].Names[0].Name
}
