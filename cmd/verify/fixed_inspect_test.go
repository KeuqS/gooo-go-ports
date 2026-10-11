package main

import (
	"encoding/json"
	"testing"
)

func TestFixedReceiptRejectsCorrelatedRewrites(t *testing.T) {
	added, err := fixedCorpus()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range added.Suites {
		raw, one, identity := contractFixture(t, s)
		changes := map[string]func(*envelope){
			"both result representations": func(e *envelope) {
				d := &e.Result.Runtime.Traces[0].Deliveries[0]
				wrong := map[string]any{}
				for k, v := range one.Cases[0].Expected.(map[string]any) {
					wrong[k] = v
				}
				key := "value"
				if one.Activity == "PixelBounds" {
					key = "min_x"
				}
				wrong[key] = int64(1234)
				d.Actual, d.Expected = mustJSON(t, wrong), mustJSON(t, wrong)
				for i, f := range d.ActualFields {
					d.ActualFields[i].Value = mustJSON(t, wrong[f.Name])
				}
			},
			"acceptance flag": func(e *envelope) {
				d := &e.Result.Runtime.Traces[0].Deliveries[0]
				wrong := map[string]any{}
				for k, v := range one.Cases[0].Expected.(map[string]any) {
					wrong[k] = v
				}
				key := "in_fixed_domain"
				if one.Activity == "PixelBounds" {
					key = "accepted"
				}
				wrong[key] = !wrong[key].(bool)
				d.Actual, d.Expected = mustJSON(t, wrong), mustJSON(t, wrong)
				d.ActualFields[0].Value = mustJSON(t, wrong[key])
			},
			"result field identity": func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].ActualFields[0].ID = "gooo://wrong" },
			"result field order":    func(e *envelope) { f := e.Result.Runtime.Traces[0].Deliveries[0].ActualFields; f[0], f[1] = f[1], f[0] },
			"generated source":      func(e *envelope) { e.Result.Composition.Source += "\n// changed" },
		}
		if s.Activity == "PixelBounds" {
			changes["raw original and field rewritten"] = func(e *envelope) {
				d := &e.Result.Runtime.Traces[0].Deliveries[0]
				r, _ := readRectangle(one.Cases[0].Inputs[1])
				r[0]++
				v := rectangleRecord(r)
				d.Inputs[1].Value = mustJSON(t, v)
				d.Inputs[1].Fields = rectangleFixtureFields(t, rectangleEntity, v)
			}
		} else {
			changes["original raw rewritten"] = func(e *envelope) { e.Result.Runtime.Traces[0].Deliveries[0].Input = mustJSON(t, int64(0)) }
		}
		for name, change := range changes {
			t.Run(s.Activity+"/"+name, func(t *testing.T) {
				var e envelope
				if err := json.Unmarshal(raw, &e); err != nil {
					t.Fatal(err)
				}
				change(&e)
				if _, err := inspect(mustJSON(t, e), one, false, identity); err == nil {
					t.Fatal("accepted mutated receipt")
				}
			})
		}
	}
}
