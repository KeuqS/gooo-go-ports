package demobridge

import (
	"strconv"
	"testing"
)

func TestDurationBridgePreservesBothSignedInt64InputsAndReturnedOutput(t *testing.T) {
	values := []int64{-9223372036854775808, -9223372036854775807, -9007199254740993, -1, 0, 1, 2, 9007199254740992, 9007199254740993, 9223372036854775807}
	for _, operation := range []string{"time.Duration.Round", "time.Duration.Truncate"} {
		for _, duration := range values {
			for _, multiple := range values {
				inputs := []string{strconv.FormatInt(duration, 10), strconv.FormatInt(multiple, 10)}
				calls := 0
				r := readResponse(t, Run(operation, inputs, func(name string, a, b, c, d int64) any {
					calls++
					if name != operation || a != duration || b != multiple || c != 0 || d != 0 {
						t.Fatalf("duration inputs changed: %s %d %d %d %d", name, a, b, c, d)
					}
					// Deliberately not a quantization result: only Gooo computes it.
					return testReply{First: -9007199254740993}
				}))
				if !r.OK || calls != 1 || r.Inputs[0] != inputs[0] || r.Inputs[1] != inputs[1] || r.Output.Type != "integer" || r.Output.Value != "-9007199254740993" {
					t.Fatalf("duration bridge altered input or output: %+v", r)
				}
			}
		}
	}
}

func TestDurationBridgeRejectsMalformedInputsBeforeComputation(t *testing.T) {
	for _, operation := range []string{"time.Duration.Round", "time.Duration.Truncate"} {
		for position := 0; position < 2; position++ {
			for _, raw := range []string{"", " 1", "1 ", "+1", "01", "-0", "1.0", "1e2", "0xff", "NaN", "１２", "9223372036854775808", "-9223372036854775809"} {
				inputs := []string{"9007199254740993", "-1"}
				inputs[position] = raw
				rejectBeforeCompute(t, operation, inputs)
			}
		}
		for n := 0; n <= 5; n++ {
			if n == 2 {
				continue
			}
			inputs := make([]string, n)
			for i := range inputs {
				inputs[i] = "0"
			}
			rejectBeforeCompute(t, operation, inputs)
		}
	}
}
