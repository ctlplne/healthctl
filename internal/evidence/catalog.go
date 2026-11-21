package evidence

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
)

//go:embed starter.json
var starterData []byte

func Starter() Pack {
	var p Pack
	if json.Unmarshal(starterData, &p) != nil || Validate(p) != nil {
		panic("invalid bundled evidence collection")
	}
	return p
}

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

func ValidID(s string) bool { return identifier.MatchString(s) && s != "." && s != ".." }
func validDate(s string) bool {
	for _, layout := range []string{"2006-01-02", "2006-01"} {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}
func webAddress(s string) bool {
	u, err := url.Parse(s)
	return err == nil && len(s) <= 1500 && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
func bounded(s string, max int) bool { return strings.TrimSpace(s) != "" && len(s) <= max }
func Validate(p Pack) error {
	if !ValidID(p.ID) || !ValidID(p.Version) || !bounded(p.Title, 150) || len(p.Passages) == 0 || len(p.Passages) > 100 {
		return errors.New("supply a collection ID, version, title and 1–100 passages")
	}
	seen := map[string]bool{}
	for _, q := range p.Passages {
		if seen[q.ID] || !ValidID(q.ID) || !bounded(q.Title, 180) || !bounded(q.Section, 220) || !bounded(q.Text, 1000) || !bounded(q.Publisher, 180) || !webAddress(q.URL) || !validDate(q.Published) || !validDate(q.Verified) || !bounded(q.Population, 350) || !bounded(q.Limitations, 850) || !bounded(q.Rights, 850) || !webAddress(q.RightsURL) {
			return errors.New("each unique passage requires dated content, publisher, HTTPS source, rights, population and applicability limits")
		}
		if q.ReviewStatus != "unreviewed_for_application" && !(q.ReviewStatus == "operator_attested" && bounded(q.ReviewRecord, 500)) {
			return errors.New("supply the review status and a review record for an operator-attested review")
		}
		if q.Status != "" || q.Collection != "" || q.Version != "" {
			return errors.New("passage lifecycle and collection identity are assigned by the application")
		}
		seen[q.ID] = true
		if a := q.Applicability; a != nil && ((a.Scope != "general_education" && a.Scope != "manual_review") || a.MinAge < 0 || a.MinAge > a.MaxAge || a.MaxAge > 130) {
			return errors.New("declare a valid population scope and age interval")
		}
		if len(q.Conditions) > 20 || len(q.Features) > 16 {
			return errors.New("too many condition or evidence links")
		}
		for _, term := range q.Conditions {
			if !bounded(term, 120) {
				return errors.New("invalid condition name")
			}
		}
		for _, f := range q.Features {
			if !bounded(f.Quote, 300) || !strings.Contains(q.Text, f.Quote) || len(f.Concepts) == 0 || len(f.Concepts) > 12 {
				return errors.New("an evidence feature must quote the passage directly and name its linked concept")
			}
			for _, term := range f.Concepts {
				if !bounded(term, 100) {
					return errors.New("invalid evidence concept")
				}
			}
		}
		if len(q.Topics) == 0 || len(q.Topics) > 20 || len(q.ConflictsWith) > 8 {
			return errors.New("supply 1–20 focused topic aliases and at most eight conflict references")
		}
		for _, top := range q.Topics {
			if !bounded(top, 120) || normalize(top) == "" {
				return errors.New("invalid topic alias")
			}
		}
		for _, ref := range q.ConflictsWith {
			parts := strings.Split(ref, "/")
			if len(parts) != 3 || !ValidID(parts[0]) || !ValidID(parts[1]) || !ValidID(parts[2]) || ref == p.ID+"/"+p.Version+"/"+q.ID {
				return errors.New("conflict references must identify a different collection/version/passage")
			}
		}
	}
	return nil
}

func Hash(data []byte) string { v := sha256.Sum256(data); return hex.EncodeToString(v[:]) }
func normalize(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

type Catalog struct {
	documents []indexedPassage
	postings  map[string]map[int]int
	Version   string
	aliases   map[string][]Passage
	byRef     map[string]Passage
}
type ActivePack struct {
	Pack Pack
	Hash string
}

func Build(packs []ActivePack) Catalog {
	c := Catalog{postings: map[string]map[int]int{}, aliases: map[string][]Passage{}, byRef: map[string]Passage{}}
	versions := []string{}
	for _, batch := range packs {
		p := batch.Pack
		versions = append(versions, p.ID+"/"+p.Version+"/"+batch.Hash)
		for _, q := range p.Passages {
			q.Collection = p.ID
			q.Version = p.Version
			q.Status = "active"
			c.byRef[q.Collection+"/"+q.Version+"/"+q.ID] = q
			c.index(q)
			for _, alias := range q.Topics {
				k := normalize(alias)
				c.aliases[k] = append(c.aliases[k], q)
			}
		}
	}
	sort.Strings(versions)
	c.Version = Hash([]byte(strings.Join(versions, "\n")))[:20]
	return c
}

// Match only a curated topic or a narrowly scoped definition/symptom question.
// Extra concepts, treatment requests and patient-specific decisions abstain.
var specificFeature = regexp.MustCompile(`^(?:can|does|do) (?:a |an |the )?(.+?) (?:cause|include|lead to) (.+)$`)
var questionLead = regexp.MustCompile(`^(?:(?:what (?:is|are) (?:the )?(?:symptoms (?:of|for) )?|what does |tell me about |describe |define |information (?:about|on) |what can cause |how would you describe ))`)

func (c Catalog) Search(query string) Result {
	out := Result{CorpusVersion: c.Version, Outcome: "unavailable", Reason: "The installed sources do not answer this question. Try a specific condition or topic.", Passages: []Passage{}, Conflicts: []string{}}
	q := normalize(query)
	if q == "" || len(query) > 512 {
		return out
	}
	if strings.HasPrefix(q, "what can cause ") {
		return out
	}
	k := questionLead.ReplaceAllString(q, "")
	k = strings.TrimSuffix(k, " mean")
	k = strings.TrimPrefix(k, "a ")
	k = strings.TrimPrefix(k, "an ")
	seen := map[string]bool{}
	add := func(p Passage) {
		ref := p.Collection + "/" + p.Version + "/" + p.ID
		if !seen[ref] {
			out.Passages = append(out.Passages, p)
			seen[ref] = true
		}
	}
	feature := ""
	if parts := specificFeature.FindStringSubmatch(q); len(parts) > 0 {
		k = parts[1]
		feature = parts[2]
	}
	for _, p := range c.aliases[k] {
		if feature != "" {
			has := slices.ContainsFunc(p.Features, func(f Feature) bool {
				return normalize(f.Quote) == feature || slices.ContainsFunc(f.Concepts, func(t string) bool { return normalize(t) == feature })
			})
			if !has {
				continue
			}
		}
		// Definitions alone do not answer a request for a symptom list.
		if strings.Contains(q, "symptoms of ") && !strings.Contains(strings.ToLower(p.Section), "symptom") && len(p.Features) == 0 {
			continue
		}
		add(p)
	}
	if len(out.Passages) == 0 {
		return out
	}
	for _, p := range slices.Clone(out.Passages) {
		for _, ref := range p.ConflictsWith {
			if other, ok := c.byRef[ref]; ok {
				add(other)
				out.Conflicts = append(out.Conflicts, p.Title+" · "+p.Section+" / "+other.Title+" · "+other.Section)
			} else {
				out.Conflicts = append(out.Conflicts, "A declared conflicting passage is unavailable in the active collections.")
			}
		}
		// A relationship declared by either source is visible from both sides.
		key := p.Collection + "/" + p.Version + "/" + p.ID
		for _, other := range c.byRef {
			if slices.Contains(other.ConflictsWith, key) {
				add(other)
				out.Conflicts = append(out.Conflicts, other.Title+" · "+other.Section+" / "+p.Title+" · "+p.Section)
			}
		}
	}
	sort.Strings(out.Conflicts)
	out.Conflicts = slices.Compact(out.Conflicts)
	slices.SortFunc(out.Passages, func(a, b Passage) int { return strings.Compare(a.Collection+"/"+a.ID, b.Collection+"/"+b.ID) })
	out.Outcome = "passages"
	out.Reason = "The quoted passage supplies general information about the topic. Its applicability limits are shown below."
	return out
}
