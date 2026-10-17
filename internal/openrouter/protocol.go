package openrouter

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
)

const decisionInstructions = `Wybierz gotową ripostę, która dokładnie odpowiada anonimowemu profilowi wiadomości w polu state. Pole category jest wiążącym typem wiadomości. Pola conversation_role, question_kind i response_mode określają rolę wypowiedzi oraz oczekiwany rodzaj odpowiedzi. Nie traktuj presence, status, opinion, agreement, disagreement, reaction, confusion ani acknowledgement jak zwykłego pytania. Każda riposta jest jednym zdaniem bez końcowego znaku interpunkcyjnego. Pierwsza część musi bezpośrednio odpowiadać typowi wiadomości, a puenta musi pasować do tonu, humoru, bezpośredniości, wulgarności i poziomu spice. Odrzuć ripostę, która błędnie nazywa wiadomość pytaniem, prośbą, planem lub argumentem. Preferuj najmocniejszą, najcelniejszą i najzabawniejszą ripostę bez powtórzenia tej samej myśli. Nie wybieraj prośby o dodatkowy kontekst.`

type decisionRequest struct {
	Model     string                 `json:"model"`
	State     messageprofile.Profile `json:"state"`
	Questions decisionQuestions      `json:"questions"`
	Provider  providerPreferences    `json:"provider"`
}

type decisionQuestions struct {
	Reply decisionChoiceQuestion `json:"reply"`
}

type decisionChoiceQuestion struct {
	Type         string           `json:"type"`
	Instructions string           `json:"instructions"`
	Criteria     decisionCriteria `json:"criteria"`
}

type decisionCriteria struct {
	Candidate0 string `json:"candidate_0"`
	Candidate1 string `json:"candidate_1"`
	Candidate2 string `json:"candidate_2"`
	Candidate3 string `json:"candidate_3"`
	Candidate4 string `json:"candidate_4"`
	Candidate5 string `json:"candidate_5"`
	Candidate6 string `json:"candidate_6"`
	Candidate7 string `json:"candidate_7"`
}

type providerPreferences struct {
	ZDR               bool   `json:"zdr"`
	DataCollection    string `json:"data_collection"`
	RequireParameters bool   `json:"require_parameters"`
}

type decisionResponse struct {
	Answers map[string]json.RawMessage `json:"answers"`
	Error   *apiError                  `json:"error"`
}

func decisionRequestFor(model string, input Input) decisionRequest {
	return decisionRequest{
		Model: model,
		State: input.Profile,
		Questions: decisionQuestions{Reply: decisionChoiceQuestion{
			Type:         "choice",
			Instructions: decisionInstructions,
			Criteria: decisionCriteria{
				Candidate0: decisionCandidate(input.CandidateType, input.Candidates[0]),
				Candidate1: decisionCandidate(input.CandidateType, input.Candidates[1]),
				Candidate2: decisionCandidate(input.CandidateType, input.Candidates[2]),
				Candidate3: decisionCandidate(input.CandidateType, input.Candidates[3]),
				Candidate4: decisionCandidate(input.CandidateType, input.Candidates[4]),
				Candidate5: decisionCandidate(input.CandidateType, input.Candidates[5]),
				Candidate6: decisionCandidate(input.CandidateType, input.Candidates[6]),
				Candidate7: decisionCandidate(input.CandidateType, input.Candidates[7]),
			},
		}},
		Provider: providerPreferences{
			ZDR:               true,
			DataCollection:    "deny",
			RequireParameters: true,
		},
	}
}

func decisionCandidate(candidateType string, text string) string {
	return "Typ odpowiedzi: " + candidateType + ". Gotowa riposta: " + text
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
