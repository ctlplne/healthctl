// Package provider implements the deployment's explicitly selected model destination.
package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	openAIBase    = "https://api.openai.com/v1"
	anthropicBase = "https://api.anthropic.com/v1"
	appVersion    = "0.2.0"
)

type Config struct {
	Kind       string `json:"kind"`
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint"`
	Structured string `json:"structured"`
}

type Metadata struct {
	Usage          Usage          `json:"usage"`
	Provider       string         `json:"provider"`
	Endpoint       string         `json:"endpoint"`
	RequestedModel string         `json:"requestedModel"`
	ReturnedModel  string         `json:"returnedModel"`
	PromptVersion  string         `json:"promptVersion"`
	SchemaVersion  string         `json:"schemaVersion"`
	AppVersion     string         `json:"appVersion"`
	Settings       map[string]any `json:"settings"`
}

type Client struct {
	remoteOnly   bool
	allowed      map[string]bool
	keyBase      string
	workspaceID  string
	secrets      map[string]string
	secretErrors map[string]error
	httpClients  map[string]*http.Client
}

func New() *Client {
	c := &Client{
		allowed:      make(map[string]bool),
		secrets:      make(map[string]string),
		secretErrors: make(map[string]error),
		httpClients: map[string]*http.Client{
			"official": networkClient(false, false),
			"https":    networkClient(true, false),
			"http":     networkClient(true, true),
		},
	}
	for _, entry := range strings.Split(os.Getenv("CLINICAL_ALLOWED_ENDPOINTS"), ",") {
		if base, err := canonicalBase(strings.TrimSpace(entry)); err == nil {
			c.allowed[base] = true
		}
	}
	c.workspaceID = strings.TrimSpace(os.Getenv("ANTHROPIC_WORKSPACE_ID"))
	c.keyBase, _ = canonicalBase(strings.TrimSpace(os.Getenv("BYOM_KEY_ENDPOINT")))
	for kind, name := range map[string]string{"openai": "OPENAI_API_KEY", "anthropic": "ANTHROPIC_API_KEY", "compatible": "BYOM_API_KEY"} {
		c.secrets[kind], c.secretErrors[kind] = readSecret(name)
	}
	return c
}

// NewRemoteOnly restricts a synthetic live evaluation to remote HTTPS APIs.
// Application clients use New so operators can explicitly configure local inference.
func NewRemoteOnly() *Client {
	c := New()
	c.remoteOnly = true
	c.httpClients["https"] = networkClient(false, false)
	return c
}

// Status exposes deployment-configured destinations and credential presence only.
func (c *Client) Status() ([]string, map[string]bool) {
	allowed := make([]string, 0, len(c.allowed))
	for base := range c.allowed {
		allowed = append(allowed, base)
	}
	slices.Sort(allowed)
	credentials := make(map[string]bool, len(c.secrets))
	for kind, value := range c.secrets {
		credentials[kind] = value != "" && c.secretErrors[kind] == nil
	}
	return allowed, credentials
}

func readSecret(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if file := os.Getenv(name + "_FILE"); file != "" {
		if value != "" {
			return "", errors.New("configure a single server-side source for " + name)
		}
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<10 {
			return "", errors.New("could not read the configured " + name + " file")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return "", errors.New("could not read the configured " + name + " file")
		}
		value = strings.TrimSpace(string(data))
	}
	if len(value) > 16<<10 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", errors.New("invalid server-side " + name)
	}
	return value, nil
}

// Destination is the base URL to display and persist for consent and provenance.
func (cfg Config) Destination() string {
	switch cfg.Kind {
	case "openai":
		return openAIBase
	case "anthropic":
		return anthropicBase
	case "compatible":
		base, _ := canonicalBase(cfg.Endpoint)
		return base
	}
	return ""
}

func (cfg Config) native() bool {
	return cfg.Structured == "native" || ((cfg.Structured == "" || cfg.Structured == "auto") && cfg.Kind != "compatible")
}

func (c *Client) Validate(cfg Config) error {
	if c.remoteOnly && (cfg.Kind == "demo" || (cfg.Kind == "compatible" && !strings.HasPrefix(cfg.Endpoint, "https://"))) {
		return errors.New("a remote HTTPS model API is required for the live evaluation")
	}
	switch cfg.Structured {
	case "", "auto", "native", "prompt":
	default:
		return errors.New("select auto, native, or prompt structured output")
	}
	switch cfg.Kind {
	case "demo":
		if cfg.Endpoint != "" {
			return errors.New("synthetic demo does not accept an endpoint")
		}
		return nil
	case "openai", "anthropic":
		if cfg.Endpoint != "" {
			return errors.New("select a compatible provider for a custom endpoint")
		}
	case "compatible":
		base, err := canonicalBase(cfg.Endpoint)
		if err != nil {
			return err
		}
		if !c.allowed[base] {
			return errors.New("add this exact base URL to CLINICAL_ALLOWED_ENDPOINTS on the server")
		}
	default:
		return errors.New("select a supported provider")
	}
	if !validModel(cfg.Model) {
		return errors.New("enter a model identifier supplied by your provider")
	}
	return nil
}

func validModel(model string) bool {
	return model != "" && len(model) <= 256 && strings.IndexFunc(model, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) < 0
}

func (c *Client) credential(cfg Config) (string, error) {
	if err := c.secretErrors[cfg.Kind]; err != nil {
		return "", err
	}
	key := c.secrets[cfg.Kind]
	switch cfg.Kind {
	case "openai":
		if key == "" {
			return "", errors.New("configure OPENAI_API_KEY or OPENAI_API_KEY_FILE on the server")
		}
	case "anthropic":
		if c.workspaceID != "" && !validModel(c.workspaceID) {
			return "", errors.New("invalid ANTHROPIC_WORKSPACE_ID")
		}
		if key == "" {
			return "", errors.New("configure ANTHROPIC_API_KEY or ANTHROPIC_API_KEY_FILE on the server")
		}
	case "compatible":
		if key != "" && c.keyBase == "" {
			return "", errors.New("set BYOM_KEY_ENDPOINT to the exact base URL allowed to receive BYOM_API_KEY")
		}
		if cfg.Destination() != c.keyBase {
			key = ""
		}
	}
	return key, nil
}

var allowedPath = regexp.MustCompile(`^(/[a-zA-Z0-9_.~-]+)*/?$`)
var allowedHost = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

func canonicalBase(raw string) (string, error) {
	u, err := url.Parse(raw)
	invalid := errors.New("use an explicit HTTP(S) base URL without credentials, query, fragment, or special path segments")
	if err != nil || len(raw) > 2048 || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, "#%\\") || !allowedPath.MatchString(u.Path) {
		return "", invalid
	}
	host := strings.ToLower(u.Hostname())
	ip, ipErr := netip.ParseAddr(host)
	if host == "" || (ipErr != nil && !allowedHost.MatchString(host)) || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return "", invalid
	}
	if host == "metadata" || strings.HasPrefix(host, "metadata.") || host == "instance-data" || strings.HasPrefix(host, "instance-data.") {
		return "", errors.New("metadata endpoints are unavailable")
	}
	if ipErr == nil && !permittedIP(ip, true, u.Scheme == "http") {
		return "", errors.New("use a private address for HTTP, or a permitted HTTPS address")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", invalid
		}
	}
	p := strings.TrimSuffix(u.Path, "/")
	if p != "" && path.Clean(p) != p {
		return "", invalid
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = p
	return u.String(), nil
}

var blockedIPs = map[netip.Addr]bool{
	netip.MustParseAddr("100.100.100.200"): true,
	netip.MustParseAddr("fd00:ec2::254"):   true,
	netip.MustParseAddr("168.63.129.16"):   true,
	netip.MustParseAddr("192.0.0.192"):     true,
}

func permittedIP(ip netip.Addr, allowPrivate, privateOnly bool) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || blockedIPs[ip] || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() || ip.Zone() != "" {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return allowPrivate
	}
	return !privateOnly && ip.IsGlobalUnicast()
}

func networkClient(allowPrivate, privateOnly bool) *http.Client {
	dialer := net.Dialer{Timeout: 8 * time.Second}
	tr := &http.Transport{
		Proxy:                  nil,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    8 * time.Second,
		ResponseHeaderTimeout:  50 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
		MaxIdleConns:           8, IdleConnTimeout: 60 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addrs) == 0 {
				return nil, errors.New("provider DNS resolution failed")
			}
			for _, ip := range addrs {
				if !permittedIP(ip, allowPrivate, privateOnly) {
					return nil, errors.New("provider resolved to an unavailable address")
				}
			}
			// Dial only these vetted IPs. TLS verification still uses the original hostname.
			for _, ip := range addrs {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
			}
			return nil, errors.New("could not connect to configured provider")
		},
	}
	return &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
}
