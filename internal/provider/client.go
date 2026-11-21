package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxInputBytes    = 48 << 10
	maxProposalBytes = 32 << 10
	maxEnvelopeBytes = 256 << 10
	maxOutputTokens  = 4096
	requestDeadline  = 75 * time.Second
)

func (c *Client) Test(ctx context.Context, cfg Config) error {
	if err := c.Validate(cfg); err != nil {
		return err
	}
	if cfg.Kind == "demo" {
		return nil
	}
	p := call{
		system: "This is a nonclinical connectivity and output-format test. Respond with the JSON object {\"status\":\"ready\"}.",
		input:  "Check the configured model connection.", schema: probeSchema, maxTokens: 512,
	}
	_, _, err := c.run(ctx, cfg, p, nil, false)
	return err
}

// Generate validates the wire contract and calls the domain's semantic validator
// before any response is returned. Input must already be minimized by the application.
func (c *Client) Generate(ctx context.Context, cfg Config, mode string, input any, validate func([]byte) error) ([]byte, Metadata, error) {
	var zero Metadata
	if err := c.Validate(cfg); err != nil {
		return nil, zero, err
	}
	if cfg.Kind == "demo" {
		return nil, zero, errors.New("synthetic demo uses its isolated fixture workflow")
	}
	if mode != "interview" && mode != "assessment" {
		return nil, zero, errors.New("unknown model task")
	}
	if validate == nil {
		return nil, zero, errors.New("a clinical proposal validator is required")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, zero, errors.New("unable to prepare the clinical input")
	}
	if len(data) > maxInputBytes {
		return nil, zero, errors.New("history exceeds the model input budget; the saved history can still be exported")
	}
	p := call{system: instructions + "\nApplication-selected task: " + mode + ".", input: string(data), schema: proposalSchema, maxTokens: maxOutputTokens}
	return c.run(ctx, cfg, p, validate, true)
}

func (c *Client) run(parent context.Context, cfg Config, p call, validate func([]byte) error, mayRepair bool) (replyBytes []byte, meta Metadata, resultErr error) {
	var zero Metadata
	started := time.Now()
	usage := Usage{}
	defer func() {
		usage.finish(started, cfg)
		meta.Usage = usage
		if meta.Settings == nil {
			meta.Settings = map[string]any{}
		}
		meta.Settings["usage"] = usage
	}()
	ctx, cancel := context.WithTimeout(parent, requestDeadline)
	defer cancel()
	key, err := c.credential(cfg)
	if err != nil {
		return nil, zero, err
	}
	var schema schemaNode
	if err := json.Unmarshal(p.schema, &schema); err != nil {
		return nil, zero, errors.New("unavailable model contract")
	}
	retryAvailable := true

	for {
		var reply []byte
		var returned string
		if cfg.Kind == "chatgpt" {
			reply, returned, err = c.chatGPTRequest(ctx, cfg, &p, &usage)
		} else {
			body, bodyErr := p.body(cfg)
			if bodyErr != nil {
				return nil, zero, errors.New("unable to prepare provider request")
			}
			reply, returned, err = c.request(ctx, cfg, key, body, &retryAvailable, &usage)
		}
		if err == nil {
			err = validateJSON(reply, &schema)
			if err == nil && validate != nil && validate(reply) != nil {
				err = errInvalid
			}
		}
		if ctx.Err() != nil {
			return nil, zero, ctx.Err()
		}
		if err == nil {
			format := "prompt"
			if cfg.native() {
				format = "native"
			}
			settings := map[string]any{"outputFormat": format, "maxOutputTokens": p.maxTokens, "requests": usage.Requests}
			if cfg.Kind == "chatgpt" {
				delete(settings, "maxOutputTokens")
				settings["maxAcceptedOutputTokens"] = p.maxTokens
				settings["maxOutputBytes"] = maxProposalBytes
				settings["maxStreamBytes"] = maxEnvelopeBytes
				settings["billing"] = "chatgpt_plan"
				if p.reasoningEffort != "" {
					settings["reasoningEffort"] = p.reasoningEffort
				}
				settings["store"] = false
				settings["stream"] = true
			}
			if cfg.Kind == "anthropic" {
				settings["apiVersion"] = anthropicVersion
			}
			return reply, Metadata{Provider: cfg.Kind, Endpoint: cfg.Destination(), RequestedModel: cfg.Model, ReturnedModel: returned, PromptVersion: "clinical-v3", SchemaVersion: "proposal-v1", AppVersion: appVersion, Settings: settings}, nil
		}
		if !mayRepair || (!errors.Is(err, errInvalid) && !errors.Is(err, errEnvelope)) {
			return nil, zero, err
		}
		mayRepair = false
		// Previous output can contain injection or fabricated facts. Recreate from
		// the original evidence without reinserting invalid output into the prompt.
		p.system += "\n" + repairInstructions
	}
}

func (c *Client) request(ctx context.Context, cfg Config, key string, body []byte, retryAvailable *bool, usage *Usage) ([]byte, string, error) {
	suffix := "/responses"
	httpClient := c.httpClients["official"]
	switch cfg.Kind {
	case "anthropic":
		suffix = "/messages"
	case "compatible":
		suffix = "/chat/completions"
		scheme, _, _ := strings.Cut(cfg.Destination(), ":")
		httpClient = c.httpClients[scheme]
	}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Destination()+suffix, bytes.NewReader(body))
		if err != nil {
			return nil, "", errors.New("invalid provider request destination")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		if cfg.Kind == "anthropic" {
			req.Header.Set("anthropic-version", anthropicVersion)
			if c.workspaceID != "" {
				req.Header.Set("anthropic-workspace-id", c.workspaceID)
			}
		}
		usage.Requests++
		resp, err := httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return nil, "", errors.New("provider connection failed; check the configured endpoint, network and TLS")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if wait, ok := retryDelay(resp); ok && *retryAvailable {
				*retryAvailable = false
				timer := time.NewTimer(wait)
				select {
				case <-timer.C:
					continue
				case <-ctx.Done():
					timer.Stop()
					return nil, "", ctx.Err()
				}
			}
			return nil, "", responseError(resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvelopeBytes+1))
		resp.Body.Close()
		if err != nil || len(data) > maxEnvelopeBytes {
			return nil, "", errors.New("provider response exceeded its limit or was interrupted")
		}
		usage.add(cfg.Kind, data)
		return unpack(cfg.Kind, data)
	}
}

func retryDelay(resp *http.Response) (time.Duration, bool) {
	switch resp.StatusCode {
	case 429, 502, 503, 504, 529:
	default:
		return 0, false
	}
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 300 * time.Millisecond, true
	}
	var delay time.Duration
	if n, err := strconv.Atoi(value); err == nil {
		if n < 0 || n > 2 {
			return 0, false
		}
		delay = time.Duration(n) * time.Second
	} else if when, err := http.ParseTime(value); err == nil {
		delay = time.Until(when)
	} else {
		return 0, false
	}
	if delay < 0 {
		delay = 0
	}
	return delay, delay <= 2*time.Second
}

func responseError(status int) error {
	switch status {
	case 401, 403:
		return errors.New("provider rejected the configured credentials or model permission")
	case 404:
		return errors.New("provider endpoint or requested model was not found")
	case 400, 422:
		return errors.New("provider rejected the request; check the model, available context and structured-output setting")
	case 429, 529:
		return errors.New("provider rate or capacity limit reached; retry later")
	}
	return fmt.Errorf("provider request failed (HTTP %d)", status)
}
