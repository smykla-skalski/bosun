package openrouter

import (
	"encoding/json"
	"math"
)

const (
	probabilitySumTolerance = 0.05
	selectedTieTolerance    = 0.01
)

var candidateIDs = [CandidateCount]string{
	"candidate_0",
	"candidate_1",
	"candidate_2",
	"candidate_3",
	"candidate_4",
	"candidate_5",
	"candidate_6",
	"candidate_7",
}

func parseDecision(answers map[string]json.RawMessage) (int, error) {
	if len(answers) != 1 {
		return 0, newFault(FaultInvalidOutput)
	}
	rawAnswer, ok := answers["reply"]
	if !ok {
		return 0, newFault(FaultInvalidOutput)
	}

	var fields map[string]json.RawMessage
	if json.Unmarshal(rawAnswer, &fields) != nil || !hasExactAnswerFields(fields) {
		return 0, newFault(FaultInvalidOutput)
	}

	var answerType string
	var choice string
	var confidence float64
	if json.Unmarshal(fields["type"], &answerType) != nil || answerType != "choice" ||
		json.Unmarshal(fields["choice"], &choice) != nil ||
		json.Unmarshal(fields["confidence"], &confidence) != nil || !validProbability(confidence) {
		return 0, newFault(FaultInvalidOutput)
	}

	choiceIndex := candidateIndex(choice)
	if choiceIndex < 0 {
		return 0, newFault(FaultInvalidOutput)
	}
	probabilities, ok := parseProbabilities(fields["probabilities"])
	if !ok {
		return 0, newFault(FaultInvalidOutput)
	}
	maximum := probabilities[0]
	for _, probability := range probabilities[1:] {
		maximum = max(maximum, probability)
	}
	if probabilities[choiceIndex]+selectedTieTolerance < maximum {
		return 0, newFault(FaultInvalidOutput)
	}
	return choiceIndex, nil
}

func hasExactAnswerFields(fields map[string]json.RawMessage) bool {
	if len(fields) != 4 {
		return false
	}
	for _, name := range [...]string{"type", "choice", "probabilities", "confidence"} {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func parseProbabilities(raw json.RawMessage) ([CandidateCount]float64, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != CandidateCount {
		return [CandidateCount]float64{}, false
	}

	var probabilities [CandidateCount]float64
	sum := 0.0
	for index, name := range candidateIDs {
		rawProbability, ok := fields[name]
		if !ok || json.Unmarshal(rawProbability, &probabilities[index]) != nil || !validProbability(probabilities[index]) {
			return [CandidateCount]float64{}, false
		}
		sum += probabilities[index]
	}
	if math.Abs(sum-1) > probabilitySumTolerance {
		return [CandidateCount]float64{}, false
	}
	return probabilities, true
}

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func candidateIndex(choice string) int {
	for index, name := range candidateIDs {
		if choice == name {
			return index
		}
	}
	return -1
}
