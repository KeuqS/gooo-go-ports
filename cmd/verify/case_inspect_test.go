package main

import (
	"encoding/json"
	"testing"
)

func TestCaseMappingReceiptsBindOriginalInputsIntegerResultsAndFields(t *testing.T) {
	added, err := caseMappingCorpus()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range added.Suites {
		// Last input retains MaxInt64, including invalid-case precedence in To.
		s.Cases = s.Cases[len(s.Cases)-1:]
		raw, one, identity := contractFixture(t, s)
		changes := map[string]func(*envelope){
			"changed expectation and result": func(e *envelope) {
				d := &e.Result.Runtime.Traces[0].Deliveries[0]
				d.Actual, d.Expected = mustJSON(t, 0), mustJSON(t, 0)
			},
			"wrong output entity": func(e *envelope) { e.Result.Program.Image.Packages[0].Exports[0].OutputType = "gooo://std/boolean" },
		}
		if s.Activity == "To" {
			changes["wrong selector entity"] = func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].Inputs[0].Entity = "gooo://std/boolean" }
			changes["narrowed selector"] = func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].Inputs[0].Value = mustJSON(t, 0) }
			changes["narrowed rune"] = func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].Inputs[1].Value = mustJSON(t, -1) }
			changes["reordered inputs"] = func(e *envelope) { v := e.Result.Runtime.Traces[0].Deliveries[0].Inputs; v[0], v[1] = v[1], v[0] }
		} else {
			changes["narrowed input"] = func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].Input = mustJSON(t, -1) }
		}
		if s.Package == "app/case-inspector" {
			changes["missing fields"] = func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].ActualFields = nil }
			changes["mapping field identity"] = func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].ActualFields[5].ID += "wrong" }
			changes["mapping field type"] = func(e *envelope) {
				e.Result.Runtime.Traces[0].Deliveries[0].ActualFields[3].Value = mustJSON(t, "9223372036854775807")
			}
			changes["reordered fields"] = func(e *envelope) { f := e.Result.Runtime.Traces[0].Deliveries[0].ActualFields; f[3], f[5] = f[5], f[3] }
			changes["scalar validity field type"] = func(e *envelope) {
				e.Result.Runtime.Traces[0].Deliveries[0].ActualFields[1].Value = mustJSON(t, "false")
			}
		} else {
			changes["scalar with record fields"] = func(e *envelope) {
				e.Result.Runtime.Traces[0].Deliveries[0].ActualFields = []recordFieldValue{{"other", "other", mustJSON(t, 0)}}
			}
		}
		for name, change := range changes {
			t.Run(s.Activity+"/"+name, func(t *testing.T) {
				var e envelope
				if err := json.Unmarshal(raw, &e); err != nil {
					t.Fatal(err)
				}
				change(&e)
				if _, err := inspect(mustJSON(t, e), one, false, identity); err == nil {
					t.Fatal("accepted altered case mapping receipt")
				}
			})
		}
	}
}
