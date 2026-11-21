package evidence

import (
	"slices"
	"sort"
	"strings"
)

type indexedPassage struct {
	passage Passage
	body    string
}

var stopWords = map[string]bool{"a": true, "an": true, "the": true, "what": true, "is": true, "are": true, "of": true, "in": true, "for": true, "and": true, "or": true, "can": true, "to": true, "does": true, "how": true}

func (c *Catalog) index(p Passage) {
	idx := len(c.documents)
	c.documents = append(c.documents, indexedPassage{p, normalize(p.Text)})
	parts := []struct {
		text   string
		weight int
	}{{p.Title, 5}, {p.Section, 3}, {p.Text, 2}, {p.Publisher, 1}}
	for _, part := range parts {
		for _, word := range strings.Fields(normalize(part.text)) {
			if stopWords[word] {
				continue
			}
			if c.postings[word] == nil {
				c.postings[word] = map[int]int{}
			}
			c.postings[word][idx] += part.weight
		}
	}
}

func (c Catalog) Browse(query string) Result {
	result := Result{CorpusVersion: c.Version, Outcome: "unavailable", Reason: "No active passage matches the search terms.", Passages: []Passage{}, Conflicts: []string{}}
	q := normalize(query)
	words := map[string]bool{}
	for _, w := range strings.Fields(q) {
		if !stopWords[w] {
			words[w] = true
		}
	}
	if len(query) > 512 || len(words) == 0 {
		return result
	}
	type score struct {
		id         int
		sum, terms int
	}
	scores := map[int]score{}
	for w := range words {
		for id, weight := range c.postings[w] {
			s := scores[id]
			s.id = id
			s.sum += weight
			s.terms++
			scores[id] = s
		}
	}
	hits := []score{}
	for id, s := range scores {
		if s.terms*2 < len(words) {
			continue
		}
		if strings.Contains(c.documents[id].body, q) {
			s.sum += 50
		}
		s.sum += 10 * s.terms
		hits = append(hits, s)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].sum != hits[j].sum {
			return hits[i].sum > hits[j].sum
		}
		a, b := c.documents[hits[i].id].passage, c.documents[hits[j].id].passage
		return a.Collection+"/"+a.ID < b.Collection+"/"+b.ID
	})
	for _, h := range hits[:min(15, len(hits))] {
		result.Passages = append(result.Passages, c.documents[h.id].passage)
	}
	if len(result.Passages) > 0 {
		result.Outcome = "passages"
		result.Reason = "Search matches in active titles, sections and quoted text. Each passage lists its source and scope."
	}
	return result
}

func (c Catalog) Explain(index int, name, term string, age int, facts map[string]string) Support {
	r := c.Search(term)
	if r.Outcome != "passages" {
		r = c.Search(name)
	}
	s := Support{ItemIndex: index, Result: r, Relation: "unavailable", ClaimLinks: []ClaimLink{}}
	if r.Outcome != "passages" {
		return s
	}
	s.Relation = "related_reading"
	s.Reason = "Related background on this topic. No passage-to-finding link has been established for the reported evidence."
	for _, p := range r.Passages {
		a := p.Applicability
		if a == nil || a.Scope != "general_education" || age < a.MinAge || age > a.MaxAge {
			continue
		}
		matches := slices.ContainsFunc(p.Conditions, func(condition string) bool {
			return normalize(condition) == normalize(name) || normalize(condition) == normalize(term)
		})
		if !matches {
			continue
		}
		for _, feature := range p.Features {
			var ids []string
			for id, concept := range facts {
				if slices.ContainsFunc(feature.Concepts, func(allowed string) bool { return normalize(allowed) == normalize(concept) }) {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 {
				sort.Strings(ids)
				s.ClaimLinks = append(s.ClaimLinks, ClaimLink{p.Collection + "/" + p.Version + "/" + p.ID, feature.Quote, ids})
			}
		}
	}
	if len(s.ClaimLinks) > 0 {
		s.Relation = "supported_context"
		s.Reason = "These exact source words describe a feature of this condition that also appears in the reviewed history. The source does not determine the diagnosis or its rank."
	}
	return s
}
