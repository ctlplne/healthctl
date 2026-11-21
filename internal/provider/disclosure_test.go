package provider

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ctlplne/healthctl/internal/clinical"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Exercise the production compatible adapter over a real loopback TCP socket.
// The endpoint receives synthetic record content and holds no credentials.
func TestCompatibleLoopbackApprovedReportOnly(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) {
		t.Skip("loopback networking unavailable in this sandbox")
	}
	if err != nil {
		t.Fatal(err)
	}
	destination := "http://" + ln.Addr().String() + "/v1"
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", destination)
	t.Setenv("BYOM_API_KEY", "")
	t.Setenv("BYOM_API_KEY_FILE", "")
	received := make(chan string, 2)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "" {
			t.Error("wrong destination or unexpected credential")
		}
		var req struct{ Messages []map[string]string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 2 {
			t.Error("unreadable chat request")
			w.WriteHeader(400)
			return
		}
		received <- req.Messages[1]["content"]
		var in clinical.Input
		if !strings.Contains(req.Messages[0]["content"], "[report:REVIEW_ID]") || json.Unmarshal([]byte(req.Messages[1]["content"]), &in) != nil || len(in.ReportEntries) != 1 {
			t.Error("missing report-reference contract or approved entry")
			w.WriteHeader(400)
			return
		}
		var response map[string]any
		json.Unmarshal([]byte(proposal), &response)
		response["action"] = "ask"
		response["question"] = "What prompted you to have this test?"
		response["reason"] = "[report:" + in.ReportEntries[0].ReviewID + "] Knowing why the report was ordered would clarify the timeline."
		raw, _ := json.Marshal(response)
		io.WriteString(w, envelope("compatible", string(raw)))
	})
	server := http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(ln)
	t.Cleanup(func() { server.Shutdown(context.Background()) })
	cfg := Config{Kind: "compatible", Model: "synthetic-loopback-api", Endpoint: destination}
	e, err := clinical.New("test", clinical.Intake{ReportFirst: true, Concern: "I am preparing for my visit.", Age: 34, NoticeVersion: clinical.NoticeVersion}, clinical.ProviderRef(cfg), "")
	if err != nil {
		t.Fatal(err)
	}
	visible := clinical.ReportRow{ID: "selected", Original: "Selected fictional marker | Trace", Locator: "Page 1, row 2", Subject: "self", Name: "Selected fictional marker", Value: "Trace"}
	doc := clinical.SourceDocument{ID: "report", Title: "LOCAL_TITLE_SENTINEL", Kind: "synthetic", BelongsTo: "self", Rows: []clinical.ReportRow{
		{ID: "header", Original: "Patient name: PRIVATE_HEADER_SENTINEL", Locator: "Page 1, row 1"}, visible,
		{ID: "other", Original: "UNSELECTED_MARKER_SENTINEL", Locator: "Page 1, row 3"},
	}}
	if err := e.AttachReport(doc, ""); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []clinical.Command{{Action: "review_report_row", DocumentID: "report", RowID: "selected", Row: visible}, {Action: "authorize_report", DocumentID: "report", RowIDs: []string{"selected"}}, {Action: "continue"}} {
		if err := e.Apply(cmd, ""); err != nil {
			t.Fatal(err)
		}
	}
	in, err := e.ModelInput("interview")
	if err != nil {
		t.Fatal(err)
	}
	c := New()
	var validated clinical.Proposal
	_, meta, err := c.Generate(t.Context(), cfg, "interview", in, func(raw []byte) error {
		if err := json.Unmarshal(raw, &validated); err != nil {
			return err
		}
		return clinical.Validate(in, validated)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Accept("interview", validated, clinical.ModelMeta{Provider: meta.Provider, ReturnedModel: meta.ReturnedModel}, ""); err != nil {
		t.Fatal(err)
	}
	if e.QuestionSource == nil || e.QuestionSource.Key != "model_report" || e.QuestionSource.RowID != "selected" || e.QuestionSource.ReviewID != in.ReportEntries[0].ReviewID || strings.Contains(e.QuestionSource.Reason, "[report:") {
		t.Fatal("validated model question lost its approved report attribution")
	}
	text := <-received
	if !strings.Contains(text, "Selected fictional marker") || !strings.Contains(text, "Trace") || strings.Contains(text, "_SENTINEL") {
		t.Fatal("selected extract missing or unselected sentinel shared")
	}
}
