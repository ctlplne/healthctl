package clinical

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The corpus is synthetic development content. See testdata/corpus/README.md.

type corpusExtraction struct {
	mustReject bool
	finding    FindingProposal
	raw        string
}

type corpusRow struct {
	file, text, source, question string
	line                         int
	expectWarn                   bool
	extractions                  []corpusExtraction
}

func (r corpusRow) String() string { return fmt.Sprintf("%s:%d %q", r.file, r.line, r.text) }

var corpusItem = regexp.MustCompile(`^(!)?(present|denied|uncertain|declined)(?:/(self|family|other))?\s+"([^"]+)"\s+([^@]+?)(?:\s*@\s*(.+))?$`)

var corpusStatus = map[string]string{"present": "reported_present", "denied": "explicitly_denied", "uncertain": "uncertain", "declined": "declined"}

func loadCorpus(t testing.TB) []corpusRow { return loadRows(t, "corpus") }

func loadRows(t testing.TB, dir string) []corpusRow {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", dir, "*.txt"))
	if err != nil || len(files) == 0 {
		t.Fatalf("corpus files are missing: %v", err)
	}
	sort.Strings(files)
	var rows []corpusRow
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, " | ", 4)
			if len(parts) < 3 {
				t.Fatalf("%s:%d needs expect | source | text | extractions", file, n+1)
			}
			r := corpusRow{file: filepath.Base(file), line: n + 1, text: strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[2]), "|"))}
			switch strings.TrimSpace(parts[0]) {
			case "warn":
				r.expectWarn = true
			case "none":
			default:
				t.Fatalf("%s:%d expect must be warn or none", file, n+1)
			}
			source := strings.TrimSpace(parts[1])
			switch {
			case source == "self" || source == "witness":
				r.source = source
			case strings.HasPrefix(source, "reply:"):
				r.source, r.question = "reply", strings.TrimSpace(strings.TrimPrefix(source, "reply:"))
			default:
				t.Fatalf("%s:%d unknown source %q", file, n+1, source)
			}
			if len(parts) == 4 {
				for _, item := range strings.Split(parts[3], ";") {
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					m := corpusItem.FindStringSubmatch(item)
					if m == nil {
						t.Fatalf("%s:%d cannot parse extraction %q", file, n+1, item)
					}
					if strings.Count(r.text, m[4]) != 1 {
						t.Fatalf("%s:%d quote %q must appear exactly once in %q", file, n+1, m[4], r.text)
					}
					subject := m[3]
					if subject == "" {
						subject = "self"
					}
					f := FindingProposal{Evidence: Evidence{AnswerID: "a1", Quote: m[4]}, Concept: strings.TrimSpace(m[5]), Status: corpusStatus[m[2]], Subject: subject, TimeContext: strings.TrimSpace(m[6])}
					r.extractions = append(r.extractions, corpusExtraction{mustReject: m[1] == "!", finding: f, raw: item})
				}
			}
			rows = append(rows, r)
		}
	}
	return rows
}

func corpusAnswer(r corpusRow) Answer {
	switch r.source {
	case "witness":
		return Answer{ID: "a1", Kind: "witness", Question: "Witness observation from Sam", Text: r.text, Active: true}
	case "reply":
		return Answer{ID: "a1", Kind: "reply", Question: r.question, Text: r.text, Active: true, Disposition: responseDisposition(r.text, r.question)}
	}
	return Answer{ID: "a1", Kind: "concern", Question: "What brings you here?", Text: r.text, Active: true}
}

// corpusWarns sends the text through the same entry points the app uses.
func corpusWarns(t testing.TB, r corpusRow) bool {
	t.Helper()
	if r.source == "self" {
		e, err := New("corpus", Intake{Concern: r.text, Age: 40, NoticeVersion: NoticeVersion}, ProviderRef{Kind: "demo", Model: "demo"}, "")
		if err != nil {
			t.Fatalf("%s: %v", r, err)
		}
		return e.hasAnyWarning()
	}
	e, err := New("corpus", Intake{Concern: "Recording an account for this visit.", Age: 40, NoticeVersion: NoticeVersion}, ProviderRef{Kind: "demo", Model: "demo"}, "")
	if err != nil || e.hasWarning() {
		t.Fatalf("neutral intake failed: %v", err)
	}
	a := corpusAnswer(r)
	e.appendDatedAnswer(a.Kind, a.Question, a.Text, "", "", "")
	return e.hasAnyWarning()
}

func corpusCheck(r corpusRow, x corpusExtraction) error {
	a := corpusAnswer(r)
	in := Input{Task: "interview", Age: 40, Concern: r.text, Answers: []Answer{a}}
	return validateFinding(in, x.finding)
}

type corpusMetrics struct {
	rows, warnRows, noneRows, missed, falseAlarms            int
	correct, correctRejected, mustReject, mustRejectAccepted int
	missedRows, alarmRows, rejectedItems, acceptedItems      []string
}

func measureCorpus(t testing.TB) corpusMetrics { return measureRows(t, loadCorpus(t)) }

func measureRows(t testing.TB, rows []corpusRow) corpusMetrics {
	var m corpusMetrics
	for _, r := range rows {
		m.rows++
		warned := corpusWarns(t, r)
		if r.expectWarn {
			m.warnRows++
			if !warned {
				m.missed++
				m.missedRows = append(m.missedRows, r.String())
			}
		} else {
			m.noneRows++
			if warned {
				m.falseAlarms++
				m.alarmRows = append(m.alarmRows, r.String())
			}
		}
		for _, x := range r.extractions {
			err := corpusCheck(r, x)
			if x.mustReject {
				m.mustReject++
				if err == nil {
					m.mustRejectAccepted++
					m.acceptedItems = append(m.acceptedItems, r.String()+" accepted "+x.raw)
				}
				continue
			}
			m.correct++
			if err != nil {
				m.correctRejected++
				m.rejectedItems = append(m.rejectedItems, r.String()+" rejected "+x.raw+": "+err.Error())
			}
		}
	}
	return m
}

func percent(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

// corpusEnforced turns on the thresholds once the shared context engine lands.
const corpusEnforced = true

func TestCorpus(t *testing.T) {
	m := measureCorpus(t)
	fmt.Printf("CORPUS rows=%d warn_rows=%d none_rows=%d\n", m.rows, m.warnRows, m.noneRows)
	fmt.Printf("CORPUS urgent_missed=%d/%d false_alarms=%d/%d (%.1f%%)\n", m.missed, m.warnRows, m.falseAlarms, m.noneRows, percent(m.falseAlarms, m.noneRows))
	fmt.Printf("CORPUS correct_rejected=%d/%d (%.1f%%) must_reject_accepted=%d/%d\n", m.correctRejected, m.correct, percent(m.correctRejected, m.correct), m.mustRejectAccepted, m.mustReject)
	if os.Getenv("CORPUS_DETAILS") != "" {
		for _, s := range m.missedRows {
			fmt.Println("MISSED", s)
		}
		for _, s := range m.alarmRows {
			fmt.Println("ALARM", s)
		}
		for _, s := range m.rejectedItems {
			fmt.Println("REJECTED", s)
		}
		for _, s := range m.acceptedItems {
			fmt.Println("ACCEPTED", s)
		}
	}
	if m.rows < 500 {
		t.Fatalf("the corpus needs at least 500 rows, found %d", m.rows)
	}
	if !corpusEnforced || os.Getenv("CORPUS_REPORT_ONLY") != "" {
		return
	}
	if m.missed != 0 {
		t.Errorf("%d urgent sentences raised no warning: %v", m.missed, m.missedRows)
	}
	if percent(m.correctRejected, m.correct) > 5 {
		t.Errorf("%.1f%% of correct extractions were rejected (limit 5%%): %v", percent(m.correctRejected, m.correct), m.rejectedItems)
	}
	if m.mustRejectAccepted != 0 {
		t.Errorf("%d extractions that must be rejected were accepted: %v", m.mustRejectAccepted, m.acceptedItems)
	}
}

// TestCorpusHoldout reports results on sentences written after the engine and
// never used for tuning. It measures how well the rules generalize.
func TestCorpusHoldout(t *testing.T) {
	if files, _ := filepath.Glob(filepath.Join("testdata", "holdout", "*.txt")); len(files) == 0 {
		t.Skip("no held-out set is waiting to be measured")
	}
	m := measureRows(t, loadRows(t, "holdout"))
	fmt.Printf("HOLDOUT rows=%d urgent_missed=%d/%d false_alarms=%d/%d\n", m.rows, m.missed, m.warnRows, m.falseAlarms, m.noneRows)
	if os.Getenv("CORPUS_DETAILS") != "" {
		for _, s := range m.missedRows {
			fmt.Println("HOLDOUT MISSED", s)
		}
		for _, s := range m.alarmRows {
			fmt.Println("HOLDOUT ALARM", s)
		}
	}
}
