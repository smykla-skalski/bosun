package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
	"github.com/smykla-skalski/bosun/internal/replies"
)

const (
	defaultBaseURL            = "https://openrouter.ai/api"
	ModelJev                  = "typesafe/jev-1.14"
	CandidateCount            = 8
	maximumResponseBytes      = 1 << 20
	maximumInFlight           = 4
	defaultRecoveryRetryDelay = 5 * time.Second
	maximumRetryDelay         = 15 * time.Minute
	decisionPath              = "/alpha/decisions"
	keyPath                   = "/v1/key"

	FaultCanceled        = "canceled"
	FaultCredit          = "credit_unavailable"
	FaultInvalidInput    = "invalid_input"
	FaultInvalidOutput   = "invalid_output"
	FaultOverload        = "overload"
	FaultRemote          = "remote_error"
	FaultTemporary       = "temporary_unavailable"
	FaultTemporaryCredit = "temporary_credit"
	FaultTimeout         = "timeout"
	FaultTransport       = "transport_error"
)

type Config struct {
	APIKey          string
	Model           string
	BalanceCooldown time.Duration
	RequestTimeout  time.Duration
}

type Input struct {
	Profile    messageprofile.Profile
	Candidates [CandidateCount]string
}

type Client struct {
	apiKey          string
	model           string
	balanceCooldown time.Duration
	requestTimeout  time.Duration
	httpClient      *http.Client
	baseURL         string
	now             func() time.Time
	requests        chan struct{}

	mu                      sync.Mutex
	creditDeniedUntil       time.Time
	temporaryDeniedUntil    time.Time
	temporaryDenyClass      string
	temporaryDenyGeneration uint64
	circuitGeneration       uint64
	recoverySequence        uint64
	recoveryOwner           uint64
}

type admission struct {
	circuitGeneration uint64
	recoveryOwner     uint64
}

type Option func(*Client)

func WithBaseURL(baseURL string) Option {
	return func(client *Client) {
		client.baseURL = strings.TrimRight(baseURL, "/")
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) {
		if httpClient != nil {
			client.httpClient = httpClient
		}
	}
}

func WithClock(now func() time.Time) Option {
	return func(client *Client) {
		if now != nil {
			client.now = now
		}
	}
}

func New(config Config, options ...Option) (*Client, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("OpenRouter API key is empty")
	}
	if !IsAllowedModel(strings.TrimSpace(config.Model)) {
		return nil, errors.New("OpenRouter model is not allowlisted")
	}
	if config.BalanceCooldown <= 0 {
		return nil, errors.New("OpenRouter balance cooldown is not positive")
	}
	if config.RequestTimeout <= 0 {
		return nil, errors.New("OpenRouter request timeout is not positive")
	}

	client := &Client{
		apiKey:            strings.TrimSpace(config.APIKey),
		model:             strings.TrimSpace(config.Model),
		balanceCooldown:   config.BalanceCooldown,
		requestTimeout:    config.RequestTimeout,
		httpClient:        &http.Client{},
		baseURL:           defaultBaseURL,
		now:               time.Now,
		requests:          make(chan struct{}, maximumInFlight),
		circuitGeneration: 1,
	}
	for _, option := range options {
		option(client)
	}
	if client.baseURL == "" {
		return nil, errors.New("OpenRouter base URL is empty")
	}
	return client, nil
}

func (c *Client) Complete(parent context.Context, input Input) (int, error) {
	if !validateInput(input) {
		return 0, newFault(FaultInvalidInput)
	}

	admitted, err := c.admit(c.now())
	if err != nil {
		return 0, err
	}
	recovery := admitted.recoveryOwner != 0
	if recovery {
		checkCtx, cancelCheck := context.WithTimeout(parent, c.requestTimeout)
		limit, checkErr := c.keyLimit(checkCtx, admitted.circuitGeneration)
		cancelCheck()
		if checkErr != nil {
			c.finishFailedRecovery(admitted, checkErr)
			return 0, checkErr
		}
		if limit != nil && *limit <= 0 {
			c.replaceCreditCircuit(admitted, c.now())
			return 0, newFault(FaultCredit)
		}
	}

	completionCtx, cancelCompletion := context.WithTimeout(parent, c.requestTimeout)
	defer cancelCompletion()
	reply, completionErr := c.complete(completionCtx, input, admitted.circuitGeneration)
	if recovery {
		c.finishRecoveryProbe(admitted, completionErr)
	}
	return reply, completionErr
}

func (c *Client) finishRecoveryProbe(admitted admission, err error) {
	switch faultClass(err) {
	case "":
		c.closeCreditCircuit(admitted)
	case FaultTemporaryCredit:
		c.finishFailedRecovery(admitted, err)
	default:
		c.replaceCreditCircuit(admitted, c.now())
	}
}

func (c *Client) admit(now time.Time) (admission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if now.Before(c.temporaryDeniedUntil) {
		return admission{}, newFault(c.temporaryDenyClass)
	}
	if c.creditDeniedUntil.IsZero() {
		return admission{circuitGeneration: c.circuitGeneration}, nil
	}
	if now.Before(c.creditDeniedUntil) {
		return admission{}, newFault(FaultCredit)
	}
	if c.recoveryOwner != 0 {
		return admission{}, newFault(FaultCredit)
	}
	c.recoverySequence++
	if c.recoverySequence == 0 {
		c.recoverySequence++
	}
	c.recoveryOwner = c.recoverySequence
	return admission{circuitGeneration: c.circuitGeneration, recoveryOwner: c.recoveryOwner}, nil
}

func (c *Client) complete(ctx context.Context, input Input, generation uint64) (int, error) {
	if err := contextFault(ctx.Err()); err != nil {
		return 0, err
	}
	select {
	case c.requests <- struct{}{}:
		defer func() { <-c.requests }()
	case <-ctx.Done():
		return 0, contextFault(ctx.Err())
	default:
		return 0, newFault(FaultOverload)
	}

	body, err := json.Marshal(decisionRequestFor(c.model, input))
	if err != nil {
		return 0, newFault(FaultInvalidInput)
	}

	response, responseBody, err := c.do(ctx, http.MethodPost, decisionPath, body, generation)
	if err != nil {
		return 0, err
	}
	var decoded decisionResponse
	decodeErr := json.Unmarshal(responseBody, &decoded)
	if statusNeedsImmediateClassification(response.StatusCode) {
		apiErr := classifyAPIError(response.StatusCode, decoded.Error)
		if apiErr != nil {
			c.applyFaultDeny(apiErr, response.Header.Get("Retry-After"), generation)
			return 0, apiErr
		}
	}
	if decodeErr != nil {
		return 0, newFault(FaultInvalidOutput)
	}
	if apiErr := classifyAPIError(response.StatusCode, decoded.Error); apiErr != nil {
		c.applyFaultDeny(apiErr, response.Header.Get("Retry-After"), generation)
		return 0, apiErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return 0, newFault(FaultRemote)
	}
	return parseDecision(decoded.Answers)
}

func validateInput(input Input) bool {
	if input.Profile.Validate() != nil {
		return false
	}
	seen := make(map[string]struct{}, CandidateCount)
	for _, candidate := range input.Candidates {
		if replies.ValidateModelCandidate(candidate) != nil {
			return false
		}
		if _, exists := seen[candidate]; exists {
			return false
		}
		seen[candidate] = struct{}{}
	}
	return true
}

func (c *Client) keyLimit(ctx context.Context, generation uint64) (*float64, error) {
	response, responseBody, err := c.do(ctx, http.MethodGet, keyPath, nil, generation)
	if err != nil {
		return nil, err
	}
	var decoded keyResponse
	decodeErr := json.Unmarshal(responseBody, &decoded)
	if statusNeedsImmediateClassification(response.StatusCode) {
		apiErr := classifyAPIError(response.StatusCode, decoded.Error)
		if apiErr != nil {
			c.applyFaultDeny(apiErr, response.Header.Get("Retry-After"), generation)
			return nil, apiErr
		}
	}
	if decodeErr != nil {
		return nil, newFault(FaultRemote)
	}
	if apiErr := classifyAPIError(response.StatusCode, decoded.Error); apiErr != nil {
		c.applyFaultDeny(apiErr, response.Header.Get("Retry-After"), generation)
		return nil, apiErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, newFault(FaultRemote)
	}
	return parseKeyLimit(decoded.Data)
}

func (c *Client) do(
	ctx context.Context,
	method string,
	path string,
	body []byte,
	generation uint64,
) (*http.Response, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, newFault(FaultTransport)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("X-OpenRouter-Cache", "false")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, nil, requestFault(ctx, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		if fault := c.faultForUnreadableStatus(response, generation); fault != nil {
			return nil, nil, fault
		}
		return nil, nil, requestFault(ctx, err)
	}
	if len(responseBody) > maximumResponseBytes {
		if fault := c.faultForUnreadableStatus(response, generation); fault != nil {
			return nil, nil, fault
		}
		return nil, nil, newFault(FaultRemote)
	}
	return response, responseBody, nil
}

func (c *Client) faultForUnreadableStatus(response *http.Response, generation uint64) error {
	fault := classifyAPIError(response.StatusCode, nil)
	switch faultClass(fault) {
	case FaultCredit, FaultTemporary:
		c.applyFaultDeny(fault, response.Header.Get("Retry-After"), generation)
		return fault
	default:
		return nil
	}
}

func (c *Client) applyFaultDeny(err error, retryAfter string, generation uint64) {
	switch faultClass(err) {
	case FaultCredit:
		c.extendCreditCircuit(generation, c.now())
	case FaultTemporary, FaultTemporaryCredit:
		now := c.now()
		delay, ok := parseRetryAfter(retryAfter, now)
		if !ok {
			delay = defaultRecoveryRetryDelay
		}
		if delay > maximumRetryDelay {
			delay = maximumRetryDelay
		}
		class := faultClass(err)
		c.mu.Lock()
		if c.circuitGeneration != generation {
			c.mu.Unlock()
			return
		}
		until := now.Add(delay)
		if until.After(c.temporaryDeniedUntil) {
			c.temporaryDeniedUntil = until
			c.temporaryDenyClass = class
			c.temporaryDenyGeneration = generation
		}
		c.mu.Unlock()
	}
}

func (c *Client) extendCreditCircuit(generation uint64, now time.Time) {
	c.mu.Lock()
	if c.circuitGeneration == generation {
		until := now.Add(c.balanceCooldown)
		if until.After(c.creditDeniedUntil) {
			c.creditDeniedUntil = until
		}
	}
	c.mu.Unlock()
}

func (c *Client) replaceCreditCircuit(admitted admission, now time.Time) {
	c.mu.Lock()
	if c.ownsRecovery(admitted) {
		until := now.Add(c.balanceCooldown)
		if until.After(c.creditDeniedUntil) {
			c.creditDeniedUntil = until
		}
		c.recoveryOwner = 0
	}
	c.mu.Unlock()
}

func (c *Client) closeCreditCircuit(admitted admission) {
	c.mu.Lock()
	if c.ownsRecovery(admitted) {
		c.creditDeniedUntil = time.Time{}
		c.recoveryOwner = 0
		if c.temporaryDenyGeneration == admitted.circuitGeneration {
			c.temporaryDeniedUntil = time.Time{}
			c.temporaryDenyClass = ""
			c.temporaryDenyGeneration = 0
		}
		c.circuitGeneration++
		if c.circuitGeneration == 0 {
			c.circuitGeneration++
		}
	}
	c.mu.Unlock()
}

func (c *Client) ownsRecovery(admitted admission) bool {
	return c.circuitGeneration == admitted.circuitGeneration && c.recoveryOwner == admitted.recoveryOwner
}

func (c *Client) finishFailedRecovery(admitted admission, err error) {
	if class := faultClass(err); class == FaultTemporary || class == FaultTemporaryCredit {
		now := c.now()
		c.mu.Lock()
		if c.ownsRecovery(admitted) && !now.Before(c.temporaryDeniedUntil) {
			c.temporaryDeniedUntil = now.Add(defaultRecoveryRetryDelay)
			c.temporaryDenyClass = class
			c.temporaryDenyGeneration = admitted.circuitGeneration
		}
		if c.ownsRecovery(admitted) {
			c.recoveryOwner = 0
		}
		c.mu.Unlock()
		return
	}
	c.replaceCreditCircuit(admitted, c.now())
}
