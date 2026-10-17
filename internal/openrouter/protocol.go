package openrouter

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
)

type completionRequest struct {
	Model               string              `json:"model"`
	Messages            []message           `json:"messages"`
	Stream              bool                `json:"stream"`
	MaxCompletionTokens int                 `json:"max_completion_tokens"`
	ReasoningEffort     string              `json:"reasoning_effort"`
	Provider            providerPreferences `json:"provider"`
	ResponseFormat      responseFormat      `json:"response_format"`
}

type providerPreferences struct {
	ZDR               bool   `json:"zdr"`
	DataCollection    string `json:"data_collection"`
	RequireParameters bool   `json:"require_parameters"`
}

type responseFormat struct {
	Type       string             `json:"type"`
	JSONSchema jsonSchemaContract `json:"json_schema"`
}

type jsonSchemaContract struct {
	Name   string       `json:"name"`
	Strict bool         `json:"strict"`
	Schema choiceSchema `json:"schema"`
}

type choiceSchema struct {
	Type                 string           `json:"type"`
	Properties           choiceProperties `json:"properties"`
	Required             []string         `json:"required"`
	AdditionalProperties bool             `json:"additionalProperties"`
}

type choiceProperties struct {
	Choice integerChoice `json:"choice"`
}

type integerChoice struct {
	Type        string              `json:"type"`
	Description string              `json:"description"`
	Enum        [CandidateCount]int `json:"enum"`
}

type decisionPayload struct {
	Profile    messageprofile.Profile            `json:"profile"`
	Candidates [CandidateCount]decisionCandidate `json:"candidates"`
}

type decisionCandidate struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string    `json:"finish_reason"`
		Error        *apiError `json:"error"`
	} `json:"choices"`
	Error *apiError `json:"error"`
}

type keyResponse struct {
	Data  json.RawMessage `json:"data"`
	Error *apiError       `json:"error"`
}

type keyData struct {
	LimitRemaining json.RawMessage `json:"limit_remaining"`
}

type apiError struct {
	Code     json.RawMessage `json:"code"`
	Metadata struct {
		LimitSource string `json:"limit_source"`
	} `json:"metadata"`
}

func classifyAPIError(status int, apiErr *apiError) error {
	credit := status == http.StatusPaymentRequired
	errorCode := 0
	if apiErr != nil {
		errorCode, _ = parseAPIErrorCode(apiErr.Code)
		credit = credit || errorCode == http.StatusPaymentRequired
	}
	limitSource := ""
	if apiErr != nil {
		limitSource = apiErr.Metadata.LimitSource
	}
	if (status == http.StatusForbidden || errorCode == http.StatusForbidden) && limitSource != "" {
		credit = true
	}
	if !credit {
		return classifyNonCreditError(status, errorCode, apiErr)
	}
	if limitSource == "openrouter_in_flight_budget" {
		return newFault(FaultTemporaryCredit)
	}
	return newFault(FaultCredit)
}

func classifyNonCreditError(status int, errorCode int, apiErr *apiError) error {
	if errorCode == http.StatusTooManyRequests || errorCode == http.StatusServiceUnavailable ||
		status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
		return newFault(FaultTemporary)
	}
	if apiErr != nil {
		return newFault(FaultRemote)
	}
	return nil
}

func statusNeedsImmediateClassification(status int) bool {
	switch status {
	case http.StatusPaymentRequired, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

func parseAPIErrorCode(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var numeric int
	if json.Unmarshal(raw, &numeric) == nil {
		return numeric, true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		code, err := strconv.Atoi(text)
		return code, err == nil
	}
	return 0, false
}

func parseKeyLimit(raw json.RawMessage) (*float64, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, newFault(FaultRemote)
	}
	var data keyData
	if json.Unmarshal(raw, &data) != nil || len(data.LimitRemaining) == 0 {
		return nil, newFault(FaultRemote)
	}
	if bytes.Equal(bytes.TrimSpace(data.LimitRemaining), []byte("null")) {
		return nil, nil
	}
	var limit float64
	if json.Unmarshal(data.LimitRemaining, &limit) != nil {
		return nil, newFault(FaultRemote)
	}
	return &limit, nil
}
