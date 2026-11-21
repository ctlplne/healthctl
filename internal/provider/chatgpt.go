package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var (
	accountKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	subscriptionSlug  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,159}$`)
	effortKey         = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,23}$`)
	requestIdentifier = regexp.MustCompile(`^req_[A-Za-z0-9_-]{1,100}$`)
)

func ValidChatGPTAccount(s string) bool   { return accountKeyPattern.MatchString(s) }
func validSubscriptionSlug(s string) bool { return subscriptionSlug.MatchString(s) }

// NewWithChatGPT resolves only the installation's explicitly selected OAuth grant.
// The resolver owns token renewal, plan permission, and account revocation checks.
func NewWithChatGPT(resolve func(context.Context, string) (string, error)) *Client {
	c := New()
	c.chatGPTToken = resolve
	return c
}

type ChatGPTModel struct {
	Slug            string   `json:"slug"`
	DisplayName     string   `json:"displayName"`
	ReasoningLevels []string `json:"reasoningLevels,omitempty"`
}

// ChatGPTError exposes diagnostic categories and fixed user-facing recovery text.
// Remote detail and error messages can echo patient data and are never propagated.
type ChatGPTError struct {
	HTTPStatus                        int
	Code, Param, RequestID, BodyShape string
	message                           string
}

func (e *ChatGPTError) Error() string { return e.message }

func (c *Client) planToken(ctx context.Context, key string) (string, error) {
	if !ValidChatGPTAccount(key) || c.chatGPTToken == nil {
		return "", &ChatGPTError{message: "Connect the ChatGPT account selected for this encounter and enable plan usage."}
	}
	token, err := c.chatGPTToken(ctx, key)
	if err != nil || len(token) == 0 || len(token) > 64<<10 || strings.IndexFunc(token, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return "", &ChatGPTError{message: "A usable plan token is unavailable for the selected ChatGPT account. Check its connection and permissions."}
	}
	return token, nil
}

func (c *Client) ChatGPTModels(ctx context.Context, accountKey string) ([]ChatGPTModel, error) {
	ctx, cancel := context.WithTimeout(ctx, requestDeadline)
	defer cancel()
	token, err := c.planToken(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	return c.planModels(ctx, token)
}

func (c *Client) planModels(ctx context.Context, token string) ([]ChatGPTModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAIBase+"/models", nil)
	if err != nil {
		return nil, errors.New("could not construct the ChatGPT model catalog request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClients["official"].Do(req)
	if err != nil {
		return nil, &ChatGPTError{message: "The selected ChatGPT account’s model catalog is unavailable. Try again later."}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, planHTTPError(res)
	}
	// The catalog can include long model-specific service instructions. Decode
	// only selection fields and never place the remote instructions in a prompt.
	const catalogLimit = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, catalogLimit+1))
	if err != nil || len(raw) > catalogLimit {
		return nil, errors.New("ChatGPT model catalog exceeded its limit or was interrupted")
	}
	var catalog struct {
		Models []struct {
			Slug       string `json:"slug"`
			Name       string `json:"display_name"`
			Visibility string `json:"visibility"`
			Effort     []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &catalog) != nil || catalog.Models == nil || len(catalog.Models) > 512 {
		return nil, errors.New("ChatGPT returned an unsupported model catalog")
	}
	out := []ChatGPTModel{}
	seen := map[string]bool{}
	for _, m := range catalog.Models {
		if m.Visibility != "list" {
			continue
		}
		if !validSubscriptionSlug(m.Slug) || strings.TrimSpace(m.Name) == "" || len(m.Name) > 160 || strings.IndexFunc(m.Name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 || seen[m.Slug] {
			return nil, errors.New("ChatGPT returned an invalid selectable model")
		}
		seen[m.Slug] = true
		item := ChatGPTModel{Slug: m.Slug, DisplayName: m.Name}
		for _, p := range m.Effort {
			if effortKey.MatchString(p.Effort) && !slices.Contains(item.ReasoningLevels, p.Effort) {
				item.ReasoningLevels = append(item.ReasoningLevels, p.Effort)
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (c *Client) chatGPTRequest(ctx context.Context, cfg Config, p *call, usage *Usage) ([]byte, string, error) {
	token, err := c.planToken(ctx, cfg.Endpoint)
	if err != nil {
		return nil, "", err
	}
	models, err := c.planModels(ctx, token)
	if err != nil {
		return nil, "", err
	}
	idx := slices.IndexFunc(models, func(m ChatGPTModel) bool { return m.Slug == cfg.Model })
	if idx < 0 {
		return nil, "", &ChatGPTError{message: "The selected model is unavailable for this ChatGPT account. Choose a model from its current catalog."}
	}
	p.reasoningEffort = ""
	for _, effort := range []string{"max", "xhigh", "high", "medium", "low", "minimal", "none"} {
		if slices.Contains(models[idx].ReasoningLevels, effort) {
			p.reasoningEffort = effort
			break
		}
	}
	body, err := p.body(cfg)
	if err != nil {
		return nil, "", errors.New("could not prepare the ChatGPT inference request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIBase+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, "", errors.New("could not construct the ChatGPT inference request")
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	usage.Requests++
	res, err := c.httpClients["official"].Do(req)
	if err != nil {
		return nil, "", &ChatGPTError{message: "The ChatGPT inference connection failed. The saved history is available to retry."}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", planHTTPError(res)
	}
	ct, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || ct != "text/event-stream" {
		return nil, "", &ChatGPTError{message: "ChatGPT did not return the expected response stream."}
	}
	return readPlanStream(res.Body, res.Header.Get("x-request-id"), p.maxTokens, usage)
}

type planWireError struct{ Code, Param string }

func planHTTPError(r *http.Response) *ChatGPTError {
	data, _ := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	var body struct {
		Error  *planWireError
		Detail json.RawMessage
	}
	shape := "unrecognized"
	var code, param string
	if json.Unmarshal(data, &body) == nil {
		if body.Error != nil {
			shape, code, param = "error", body.Error.Code, body.Error.Param
		} else if body.Detail != nil {
			shape = "detail"
		}
	}
	return planError(r.StatusCode, shape, code, param, r.Header.Get("x-request-id"))
}

func planError(status int, shape, code, param, id string) *ChatGPTError {
	e := &ChatGPTError{HTTPStatus: status, BodyShape: shape}
	if requestIdentifier.MatchString(id) {
		e.RequestID = id
	}
	switch param {
	case "model", "input", "instructions", "text", "text.format", "stream", "store", "reasoning", "reasoning.effort", "service_tier", "tools":
		e.Param = param
	}
	switch code {
	case "subscription_sharing_usage_limit_exceeded":
		e.message = "This ChatGPT plan usage limit has been reached. Check Usage in ChatGPT settings before retrying."
	case "subscription_sharing_usage_unavailable", "subscription_sharing_user_unavailable":
		e.message = "ChatGPT plan usage is temporarily unavailable. Keep the account connected and retry later."
	case "subscription_sharing_user_not_eligible", "chatpass_v2_scope_not_authorized", "chatpass_v2_invalid_authorization_context":
		e.message = "The selected ChatGPT account or workspace has not authorized plan usage for this application."
	case "subscription_sharing_route_not_supported", "subscription_sharing_unsupported_capability":
		e.message = "The ChatGPT plan route rejected a requested capability. Check the model and format selected in settings."
	case "subscription_sharing_invalid_user":
		e.message = "ChatGPT could not verify the selected subscriber. Check the account connection and plan permission."
	default:
		switch status {
		case 401, 403:
			e.message = "ChatGPT denied access. Check the selected account, plan permissions, and availability for this installation."
		case 429:
			e.message = "ChatGPT limited this request. Check plan usage and retry later."
		case 500, 502, 503, 504:
			e.message = "ChatGPT plan inference is temporarily unavailable. Retry later."
		default:
			e.message = "ChatGPT could not complete this request. Check the selected account and model settings."
		}
		return e
	}
	e.Code = code
	return e
}
