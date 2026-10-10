package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestDurationReceiptKeepsBothOriginalSignedInputs(t *testing.T) {
	for _, value := range []int64{math.MinInt64, math.MaxInt64, -9007199254740993, 9007199254740993} {
		for position := 0; position < 2; position++ {
			inputs := []any{int64(1500000000), int64(1000000000)}
			inputs[position] = value
			duration, quantum := inputs[0].(int64), inputs[1].(int64)
			for _, operation := range []string{"DurationRound", "DurationTruncate", "Inspect"} {
				raw, _, identity := fixture(t)
				var original envelope
				if err := json.Unmarshal(raw, &original); err != nil {
					t.Fatal(err)
				}
				s := suite{Package: "go/time", Activity: operation}
				var want any = int64(time.Duration(duration).Round(time.Duration(quantum)))
				if operation == "DurationTruncate" {
					want = int64(time.Duration(duration).Truncate(time.Duration(quantum)))
				}
				if operation == "Inspect" {
					s.Package = "app/duration-quantizer"
					var err error
					want, err = durationQuantizerOracle(duration, quantum)
					if err != nil {
						t.Fatal(err)
					}
				}
				s.Cases = []testCase{{Inputs: inputs, Expected: want}}
				contract, err := contractFor(s)
				if err != nil {
					t.Fatal(err)
				}
				identity.Sources = fixtureSources(t, s)
				original.Result.Program.Entry.Package, original.Result.Program.Entry.Activity = s.Package, s.Activity
				original.Result.Program.Image = fixtureWorkspace(t, s, identity.Sources)
				original.Result.Composition.Steps[0].Generation.Report.ID = contract.ID
				delivery := &original.Result.Runtime.Traces[0].Deliveries[0]
				delivery.ID = contract.ID
				delivery.Inputs[0].Value, delivery.Inputs[1].Value = mustJSON(t, duration), mustJSON(t, quantum)
				delivery.Actual, delivery.Expected = mustJSON(t, want), mustJSON(t, want)
				valid := mustJSON(t, original)
				if _, err := inspect(valid, s, false, identity); err != nil {
					t.Fatalf("valid %s input%d=%d: %v", operation, position, value, err)
				}
				for name, changed := range map[string]json.RawMessage{
					"replaced": mustJSON(t, int64(1)),
					"rounded":  mustJSON(t, float64(value)),
					"string":   mustJSON(t, string(mustJSON(t, value))),
					"swapped":  mustJSON(t, inputs[1-position]),
				} {
					var e envelope
					if err := json.Unmarshal(valid, &e); err != nil {
						t.Fatal(err)
					}
					e.Result.Runtime.Traces[0].Deliveries[0].Inputs[position].Value = changed
					if equalValue(changed, value) {
						t.Fatalf("mutation %s did not change original %d", name, value)
					}
					if _, err := inspect(mustJSON(t, e), s, false, identity); err == nil {
						t.Fatalf("accepted %s %s input%d=%d", name, operation, position, value)
					}
				}
				// Mutating both receipt actual and expected must still fail the
				// independent oracle, including a delta field in the consumer.
				var e envelope
				if err := json.Unmarshal(valid, &e); err != nil {
					t.Fatal(err)
				}
				bad := any("incorrect scalar result")
				if operation == "Inspect" {
					badResult, err := durationQuantizerOracle(duration, quantum)
					if err != nil {
						t.Fatal(err)
					}
					badResult.(map[string]any)["round_adjustment"] = "incorrect delta"
					bad = badResult
				}
				e.Result.Runtime.Traces[0].Deliveries[0].Actual = mustJSON(t, bad)
				e.Result.Runtime.Traces[0].Deliveries[0].Expected = mustJSON(t, bad)
				if _, err := inspect(mustJSON(t, e), s, false, identity); err == nil {
					t.Fatal("accepted changed result and expectation")
				}
			}
		}
	}
}
