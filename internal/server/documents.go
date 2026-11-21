package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/documents"
	"github.com/ctlplne/healthctl/internal/store"
)

// Bound expensive local readers independently of concurrent web sessions.
var readerSlots = make(chan struct{}, 2)

func (s *Server) documentRoutes(m *http.ServeMux) {
	s.route(m, "POST /api/encounters/{id}/documents", true, s.upload)
	s.route(m, "GET /api/encounters/{id}/documents/{doc}/original", true, s.original)
	s.route(m, "GET /api/encounters/{id}/documents/{doc}/page/{number}", true, s.documentPage)
	s.route(m, "POST /api/encounters/{id}/documents/{doc}/preview", true, s.preview)
	s.route(m, "GET /api/timeline", true, s.timeline)
	s.route(m, "GET /api/region", true, s.region)
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request, owner int) error {
	rows, err := s.Store.Timeline(r.Context(), owner)
	if err != nil {
		return err
	}
	emit(w, 200, object{"encounters": rows})
	return nil
}

func (s *Server) region(w http.ResponseWriter, r *http.Request, _ int) error {
	emit(w, 200, object{"locale": s.Locale, "units": s.Units, "emergency": s.Emergency, "careDestination": s.CareDestination})
	return nil
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request, owner int) error {
	id := r.PathValue("id")
	if _, err := s.Store.Get(r.Context(), owner, id); err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, documents.MaxFile+(64<<10))
	if err := r.ParseMultipartForm(documents.MaxFile + (64 << 10)); err != nil {
		return bad("Choose a file of up to 12 MiB and submit the form again.")
	}
	defer r.MultipartForm.RemoveAll()
	revision, err := strconv.Atoi(r.FormValue("revision"))
	key := r.FormValue("requestId")
	if err != nil || revision < 1 || !requestID.MatchString(key) {
		return bad("A revision and submission identifier are required.")
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 || files[0].Size > documents.MaxFile {
		return bad("Choose one file of up to 12 MiB.")
	}
	file, err := files[0].Open()
	if err != nil {
		return bad("Cannot open this upload.")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, documents.MaxFile+1))
	if err != nil || len(data) > documents.MaxFile {
		return bad("The upload is incomplete or too large.")
	}
	title := filepath.Base(strings.ReplaceAll(files[0].Filename, "\\", "/"))
	if len(title) > 200 || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return bad("Choose a shorter file name without control characters.")
	}
	belongs := r.FormValue("belongsTo")
	if belongs == "" {
		belongs = "unknown"
	}
	doc := clinical.SourceDocument{ID: token(16), Title: title, BelongsTo: belongs, Purpose: r.FormValue("purpose"), Priority: r.FormValue("priority"), IssueDate: r.FormValue("issueDate"), CreatedAt: time.Now().UTC()}
	doc.MedicationLabel = r.FormValue("medicationLabel") == "true"
	sum := sha256.Sum256(data)
	parts := []any{revision, key, title, belongs, doc.Purpose, doc.Priority, doc.IssueDate, hex.EncodeToString(sum[:])}
	if doc.MedicationLabel {
		parts = append(parts, "medication_label")
	}
	digest, err := s.Store.Digest(parts)
	if err != nil {
		return err
	}
	prior, err := s.Store.Uploaded(r.Context(), owner, id, key, digest)
	if err != nil {
		return err
	}
	if prior != nil {
		emit(w, 200, prior)
		return nil
	}
	select {
	case readerSlots <- struct{}{}:
		defer func() { <-readerSlots }()
	default:
		return problem{429, "The local reader is busy. Your file has not been submitted. Try again."}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	doc.Kind, doc.Rows, err = documents.Read(ctx, title, data)
	if err != nil {
		return bad(err.Error())
	}
	if doc.MedicationLabel {
		documents.LabelCandidates(doc.Rows)
	}
	e, err := s.Store.AttachDocument(r.Context(), owner, id, revision, key, digest, doc, data, s.Emergency)
	if errors.Is(err, store.ErrDocumentInput) {
		return bad(err.Error())
	}
	if err != nil {
		return err
	}
	emit(w, 200, e)
	return nil
}

func (s *Server) original(w http.ResponseWriter, r *http.Request, owner int) error {
	id, doc := r.PathValue("id"), r.PathValue("doc")
	e, err := s.Store.Get(r.Context(), owner, id)
	if err != nil {
		return err
	}
	for _, d := range e.Documents {
		if d.ID != doc {
			continue
		}
		raw, err := s.Store.Original(r.Context(), owner, id, doc)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.Title}))
		_, err = w.Write(raw)
		return err
	}
	return problem{404, "This document could not be found."}
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		RowIDs []string `json:"rowIds"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	e, err := s.Store.Get(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		return err
	}
	extracts, err := e.ReportPreview(r.PathValue("doc"), in.RowIDs)
	if err != nil {
		return bad(err.Error())
	}
	emit(w, 200, object{"extracts": extracts})
	return nil
}

func (s *Server) documentPage(w http.ResponseWriter, r *http.Request, owner int) error {
	id, doc := r.PathValue("id"), r.PathValue("doc")
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		return bad("Choose a page number.")
	}
	e, err := s.Store.Get(r.Context(), owner, id)
	if err != nil {
		return err
	}
	for _, d := range e.Documents {
		if d.ID != doc {
			continue
		}
		select {
		case readerSlots <- struct{}{}:
			defer func() { <-readerSlots }()
		default:
			return problem{429, "The local page reader is busy. Try again."}
		}
		data, err := s.Store.Original(r.Context(), owner, id, doc)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		view, err := documents.Preview(ctx, d.Kind, data, n)
		if err != nil {
			return bad(err.Error())
		}
		w.Header().Set("Content-Type", "image/png")
		_, err = w.Write(view)
		return err
	}
	return problem{404, "This document could not be found."}
}
