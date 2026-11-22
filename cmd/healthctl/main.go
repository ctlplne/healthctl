package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ctlplne/healthctl/internal/ehr"
	"github.com/ctlplne/healthctl/internal/provider"
	"github.com/ctlplne/healthctl/internal/server"
	"github.com/ctlplne/healthctl/internal/store"
	"golang.org/x/text/language"
)

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil {
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	logger := log.New(os.Stderr, "healthctl ", log.LstdFlags|log.LUTC)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	key, err := base64.StdEncoding.DecodeString(os.Getenv("CLINICAL_DATA_KEY"))
	if err != nil || len(key) != 32 {
		logger.Fatal("invalid installation data key")
	}
	open, cancel := context.WithTimeout(ctx, 20*time.Second)
	db, err := store.Open(open, os.Getenv("DATABASE_URL"), key)
	cancel()
	if err != nil {
		logger.Fatal("database initialization or installation key verification failed")
	}
	defer db.DB.Close()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check-key":
			return
		case "recover-owner", "rotate-key":
			secret, readErr := io.ReadAll(io.LimitReader(os.Stdin, 256))
			if readErr != nil {
				logger.Fatal("could not read maintenance input")
			}
			secret = bytes.TrimSuffix(secret, []byte("\n"))
			if os.Args[1] == "rotate-key" {
				var next []byte
				next, err = base64.StdEncoding.DecodeString(string(secret))
				if err == nil {
					err = db.Rotate(ctx, next)
				}
			} else {
				err = db.RecoverOwner(ctx, secret)
			}
			if err != nil {
				logger.Fatal("maintenance failed: ", err)
			}
			logger.Print("event=maintenance_completed")
			return
		default:
			logger.Fatal("unknown operation")
		}
	}
	runtimeLock, err := db.HoldRuntime(ctx)
	if err != nil {
		logger.Fatal("cannot start during maintenance")
	}
	defer runtimeLock.Close()
	if err = db.RecoverPending(ctx); err != nil {
		logger.Fatal("encounter recovery failed")
	}
	origin := os.Getenv("CLINICAL_PUBLIC_ORIGIN")
	secure := os.Getenv("CLINICAL_SECURE_COOKIE") == "true"
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !secure {
			logger.Fatal("public origin requires an HTTPS origin and secure cookies")
		}
	}
	locale, err := language.Parse(env("CLINICAL_LOCALE", "en"))
	base, _ := locale.Base()
	if err != nil || base.String() != "en" {
		logger.Fatal("CLINICAL_LOCALE must be an English locale, such as en-US or en-IN")
	}
	units := env("CLINICAL_UNITS", "as-reported")
	switch units {
	case "as-reported", "metric", "us-customary":
	default:
		logger.Fatal("CLINICAL_UNITS must be as-reported, metric or us-customary")
	}
	portal, err := server.NewEHRIntegration(ehr.Config{
		Name: os.Getenv("CLINICAL_EHR_NAME"), FHIRBase: os.Getenv("CLINICAL_EHR_FHIR_BASE"),
		AuthorizationEndpoint: os.Getenv("CLINICAL_EHR_AUTHORIZE_URL"), TokenEndpoint: os.Getenv("CLINICAL_EHR_TOKEN_URL"),
		ClientID: os.Getenv("CLINICAL_EHR_CLIENT_ID"), RedirectURI: os.Getenv("CLINICAL_EHR_REDIRECT_URI"),
		AuthorizationIssuer: os.Getenv("CLINICAL_EHR_AUTH_ISSUER"),
	})
	if err != nil {
		logger.Fatal("invalid health portal configuration: ", err)
	}
	models := provider.New()
	var plan *server.ChatGPTIntegration
	if origin == "" && !secure {
		plan = server.NewChatGPTIntegration(db)
		models = provider.NewWithChatGPT(plan.Token)
	}
	app := &server.Server{Lifetime: ctx, Store: db, Models: models, ChatGPT: plan, EHR: portal, Name: env("CLINICAL_DISPLAY_NAME", "healthctl"), Emergency: os.Getenv("CLINICAL_EMERGENCY_CONTACT"), Locale: locale.String(), Units: units, CareDestination: os.Getenv("CLINICAL_CARE_DESTINATION"), Assets: env("CLINICAL_ASSETS", "web/dist"), PublicOrigin: origin, Secure: secure, Logger: logger}
	httpd := &http.Server{Addr: env("CLINICAL_LISTEN", "127.0.0.1:8437"), Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: provider.PlanClinicalDeadline + 30*time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpd.Shutdown(deadline)
	}()
	logger.Print("event=server_ready")
	if err = httpd.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal("server stopped unexpectedly")
	}
}
