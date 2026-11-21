package clinical

func (e *Encounter) disclosed() map[string]ReportExtract {
	rows := map[string]ReportExtract{}
	for _, r := range e.ReportReviews {
		if r.Active && r.ApprovedFor != nil && *r.ApprovedFor == e.Provider {
			d := e.document(r.DocumentID)
			if d != nil && d.BelongsTo == "self" {
				rows[r.ID] = extract(r)
			}
		}
	}
	return rows
}

func allApproved(ids []string, allowed map[string]ReportExtract) bool {
	for _, id := range ids {
		if _, ok := allowed[id]; !ok {
			return false
		}
	}
	return true
}

// inputHistory omits unselected contents, source-dependent answers, questions,
// derived findings, warning quotations and contradictory concepts. Original
// documents and locators outside the approved extracts are never serialized.
func (e *Encounter) inputHistory(task string) Input {
	allowed := e.disclosed()
	in := Input{ReturnVisit: e.ReturnSource != "", Agenda: e.Agenda, Focused: e.Focused, ReviewOnly: e.ReviewExtraction, Task: task, Age: e.Age, Concern: e.Concern, Background: e.Background, HistoryRevision: e.HistoryRevision, Answers: []Answer{}, Facts: []Fact{}, ActiveWarnings: []Warning{}, Contradictions: []Contradiction{}, Outstanding: []string{}}
	for _, topic := range e.remainingBackground() {
		in.BackgroundQuestions = append(in.BackgroundQuestions, topic.question)
	}
	shownAnswers := map[string]bool{}
	for _, a := range e.Answers {
		if a.Active && !a.SourceOutdated && allApproved(a.ReviewIDs, allowed) {
			if a.Detail != nil {
				detail := *a.Detail
				detail.DiaryID = ""
				detail.PriorEncounter = ""
				detail.PriorAnswer = ""
				a.Detail = &detail
			}
			in.Answers = append(in.Answers, a)
			shownAnswers[a.ID] = true
		}
	}
	if e.ScopeReason == "" {
		in.Pathways = matchPathways(in.Answers)
	}
	shownFacts := map[string]bool{}
	for _, f := range e.Facts {
		if !f.Active {
			continue
		}
		switch {
		case f.SourceType == "report_entry":
			x, ok := allowed[f.ReviewID]
			if !ok {
				continue
			}
			in.ReportEntries = append(in.ReportEntries, x)
		case f.AnswerID != "" && !shownAnswers[f.AnswerID]:
			continue
		}
		in.Facts = append(in.Facts, f)
		shownFacts[f.ID] = true
	}
	for _, c := range e.Contradictions {
		visible := true
		for _, id := range c.FactIDs {
			if !shownFacts[id] {
				visible = false
				break
			}
		}
		if visible {
			in.Contradictions = append(in.Contradictions, c)
		}
	}
	if allApproved(e.OutstandingReviewIDs, allowed) {
		in.Outstanding = e.Outstanding
	}
	for _, w := range e.Warnings {
		if !w.Active {
			continue
		}
		if w.DocumentID != "" {
			r := e.currentReview(w.DocumentID, w.RowID)
			if r == nil {
				continue
			}
			x, ok := allowed[r.ID]
			if !ok {
				continue
			}
			w.Evidence = Evidence{Quote: x.Quote}
			if w.Rule == "report_review_comment" {
				w.Quote = x.Note
			}
		} else if w.AnswerID != "" && !shownAnswers[w.AnswerID] {
			continue
		}
		in.ActiveWarnings = append(in.ActiveWarnings, w)
	}
	for _, f := range e.Facts {
		if f.Corrected && f.Supersedes != "" && f.SourceType != "report_entry" {
			for _, old := range e.Facts {
				if old.ID == f.Supersedes && shownAnswers[old.AnswerID] {
					in.CorrectedEvidence = append(in.CorrectedEvidence, old.Evidence)
				}
			}
		}
	}
	return in
}

func inputReviews(in Input) []string {
	var ids []string
	for _, x := range in.ReportEntries {
		ids = append(ids, x.ReviewID)
	}
	return ids
}
