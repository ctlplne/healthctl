package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ctlplne/healthctl/internal/clinical"
)

var ErrDocumentInput = errors.New("document could not be attached")

func documentAAD(owner int, encounter, doc string) []byte {
	return []byte(fmt.Sprintf("document/v1/%d/%s/%s", owner, encounter, doc))
}

// AttachDocument commits the encrypted original, encounter metadata and submission
// receipt together. A retried upload cannot leave a second copy behind.
func (s *Store) AttachDocument(ctx context.Context, owner int, id string, revision int, request string, digest []byte, doc clinical.SourceDocument, original []byte, emergency string) (*clinical.Encounter, error) {
	e, _, err := s.apply(ctx, owner, id, "", revision, request, digest, func(tx *sql.Tx, e, _ *clinical.Encounter) error {
		if err := e.AttachReport(doc, emergency); err != nil {
			return fmt.Errorf("%w: %s", ErrDocumentInput, err)
		}
		sealed := s.box.Seal(nil, nil, original, documentAAD(owner, id, doc.ID))
		_, err := tx.ExecContext(ctx, "INSERT INTO encounter_documents(encounter_id,document_id,original) VALUES($1,$2,$3)", id, doc.ID, sealed)
		return err
	})
	return e, err
}

func (s *Store) Original(ctx context.Context, owner int, id, doc string) ([]byte, error) {
	var sealed []byte
	err := s.DB.QueryRowContext(ctx, `SELECT d.original FROM encounter_documents d JOIN encounters e ON e.id=d.encounter_id
         WHERE e.owner_id=$1 AND e.id=$2 AND d.document_id=$3`, owner, id, doc).Scan(&sealed)
	if err != nil {
		return nil, err
	}
	return s.box.Open(nil, nil, sealed, documentAAD(owner, id, doc))
}

func (s *Store) Timeline(ctx context.Context, owner int) ([]*clinical.Encounter, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,payload FROM encounters WHERE owner_id=$1 ORDER BY updated_at DESC,id", owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []*clinical.Encounter{}
	for rows.Next() {
		var id string
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		e, err := s.decode(owner, id, raw)
		if err != nil {
			return nil, err
		}
		// Timeline search needs document titles, dated outcomes and active facts.
		// Keep full documents, earlier assessments and transcripts in their own encounter.
		compact := &clinical.Encounter{ID: e.ID, Concern: e.Concern, EpisodeID: e.EpisodeID, EpisodeTitle: e.EpisodeTitle, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, Demo: e.Demo, Outcomes: e.Outcomes, Facts: []clinical.Fact{}}
		for _, d := range e.Documents {
			compact.Documents = append(compact.Documents, clinical.SourceDocument{ID: d.ID, Title: d.Title, Purpose: d.Purpose})
		}
		for _, f := range e.Facts {
			if f.Active {
				compact.Facts = append(compact.Facts, f)
			}
		}
		result = append(result, compact)
	}
	return result, rows.Err()
}

// Uploaded reports can be expensive to extract. A lost upload response can be
// replayed from its committed receipt before running PDF or OCR readers again.
func (s *Store) Uploaded(ctx context.Context, owner int, id, request string, digest []byte) (*clinical.Encounter, error) {
	var previous, payload []byte
	err := s.DB.QueryRowContext(ctx, `SELECT a.digest,e.payload FROM submissions a JOIN encounters e ON a.encounter_id=e.id
        WHERE e.owner_id=$1 AND e.id=$2 AND a.request_id=$3`, owner, id, request).Scan(&previous, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(previous, digest) {
		return nil, ErrConflict
	}
	return s.decode(owner, id, payload)
}
