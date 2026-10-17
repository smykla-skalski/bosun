package openrouter

import (
	"encoding/json"
	"testing"
)

func TestParseDecisionRejectsInvalidTypedAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]any, map[string]any, map[string]any)
	}{
		{name: "extra answer", mutate: func(_ map[string]any, answers map[string]any, _ map[string]any) {
			answers["other"] = map[string]any{}
		}},
		{name: "missing field", mutate: func(reply map[string]any, _ map[string]any, _ map[string]any) {
			delete(reply, "confidence")
		}},
		{name: "extra field", mutate: func(reply map[string]any, _ map[string]any, _ map[string]any) {
			reply["text"] = "not allowed"
		}},
		{name: "wrong type", mutate: func(reply map[string]any, _ map[string]any, _ map[string]any) {
			reply["type"] = "score"
		}},
		{name: "unknown choice", mutate: func(reply map[string]any, _ map[string]any, _ map[string]any) {
			reply["choice"] = "candidate_8"
		}},
		{name: "missing probability", mutate: func(_ map[string]any, _ map[string]any, probabilities map[string]any) {
			delete(probabilities, "candidate_7")
		}},
		{name: "extra probability", mutate: func(_ map[string]any, _ map[string]any, probabilities map[string]any) {
			probabilities["candidate_8"] = 0
		}},
		{name: "invalid probability", mutate: func(_ map[string]any, _ map[string]any, probabilities map[string]any) {
			probabilities["candidate_0"] = -0.1
		}},
		{name: "invalid sum", mutate: func(_ map[string]any, _ map[string]any, probabilities map[string]any) {
			probabilities["candidate_0"] = 0.5
		}},
		{name: "choice is not most probable", mutate: func(_ map[string]any, _ map[string]any, probabilities map[string]any) {
			probabilities["candidate_3"] = 0.4
			probabilities["candidate_4"] = 0.6
		}},
		{name: "invalid confidence", mutate: func(reply map[string]any, _ map[string]any, _ map[string]any) {
			reply["confidence"] = 1.1
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			reply, answers, probabilities := validTypedAnswer()
			test.mutate(reply, answers, probabilities)
			raw, err := json.Marshal(answers)
			if err != nil {
				t.Fatalf("marshal answer: %v", err)
			}
			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("decode answer: %v", err)
			}
			if choice, err := parseDecision(decoded); choice != 0 || SafeFaultClass(err) != FaultInvalidOutput {
				t.Fatalf("parseDecision() = %d, %q; want invalid output", choice, SafeFaultClass(err))
			}
		})
	}
}

func validTypedAnswer() (map[string]any, map[string]any, map[string]any) {
	probabilities := make(map[string]any, CandidateCount)
	for _, candidateID := range candidateIDs {
		probabilities[candidateID] = 0.0
	}
	probabilities["candidate_3"] = 1.0
	reply := map[string]any{
		"type":          "choice",
		"choice":        "candidate_3",
		"probabilities": probabilities,
		"confidence":    1.0,
	}
	return reply, map[string]any{"reply": reply}, probabilities
}
