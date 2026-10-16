package openrouter

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

func parseDecision(value string) (int, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return 0, newFault(FaultInvalidOutput)
	}
	key, err := decoder.Token()
	if err != nil || key != "choice" {
		return 0, newFault(FaultInvalidOutput)
	}
	rawChoice, err := decoder.Token()
	if err != nil {
		return 0, newFault(FaultInvalidOutput)
	}
	number, ok := rawChoice.(json.Number)
	if !ok {
		return 0, newFault(FaultInvalidOutput)
	}
	choice, err := strconv.Atoi(number.String())
	if err != nil || choice < 0 || choice >= CandidateCount {
		return 0, newFault(FaultInvalidOutput)
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return 0, newFault(FaultInvalidOutput)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return 0, newFault(FaultInvalidOutput)
	}
	return choice, nil
}
