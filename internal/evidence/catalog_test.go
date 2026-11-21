package evidence

import "testing"

func TestCuratedCoverageAndAbstention(t *testing.T) {
	p := Starter()
	cat := Build([]ActivePack{{Pack: p, Hash: "bundled-test"}})
	for _, topic := range []string{"What is a common cold?", "What are the symptoms of migraine?", "abdominal bloating", "dizziness", "hay fever", "reference ranges"} {
		r := cat.Search(topic)
		if r.Outcome != "passages" || len(r.Passages) == 0 || r.CorpusVersion == "" {
			t.Fatalf("unavailable for %s", topic)
		}
		for _, q := range r.Passages {
			if q.ReviewStatus != "unreviewed_for_application" || q.Collection == "" || q.Version == "" || q.Status != "active" {
				t.Fatal("citation lost provenance")
			}
		}
	}
	for _, q := range []string{"what medicine should I take for migraine", "is my chest pain dangerous", "dizziness with fainting and chest discomfort", "what are the symptoms of a reference range", "what can cause dizziness"} {
		if r := cat.Search(q); r.Outcome != "unavailable" || len(r.Passages) != 0 {
			t.Fatalf("unsupported query returned a citation: %s", q)
		}
	}
}
func TestConflictRelationsAndImmutableVersions(t *testing.T) {
	a := Starter()
	b := Starter()
	b.ID = "operator-pack"
	b.Version = "2"
	b.Passages = b.Passages[:1]
	b.Passages[0].ConflictsWith = []string{a.ID + "/" + a.Version + "/cold-definition"}
	cat := Build([]ActivePack{{Pack: a, Hash: "a"}, {Pack: b, Hash: "b"}})
	r := cat.Search("common cold")
	if len(r.Conflicts) == 0 || len(r.Passages) != 3 {
		t.Fatal("declared conflict disappeared")
	}
	if Build([]ActivePack{{Pack: a, Hash: "a"}}).Version == cat.Version {
		t.Fatal("index did not track contents")
	}
	b.Passages[0].RightsURL = ""
	if Validate(b) == nil {
		t.Fatal("unlicensed import accepted")
	}
}

func TestBrowseAndCitedClaimsAreDifferent(t *testing.T) {
	p := Starter()
	c := Build([]ActivePack{{Pack: p, Hash: "test"}})
	hits := c.Browse("throbbing or pulsing")
	if len(hits.Passages) == 0 || hits.Passages[0].ID != "migraine-pattern" {
		t.Fatal("ranked body phrase search unavailable")
	}
	if c.Search("Could throbbing mean I have a stroke?").Outcome != "unavailable" {
		t.Fatal("loose surface text answered clinical question")
	}
	if c.Search("Can allergic rhinitis cause sneezing?").Outcome != "passages" || c.Search("Can allergic rhinitis cause bone pain?").Outcome != "unavailable" {
		t.Fatal("feature question lacks direct evidence")
	}
	s := c.Explain(0, "nasal allergies", "allergic rhinitis", 34, map[string]string{"f7": "Sneezing"})
	if s.Relation != "supported_context" || len(s.ClaimLinks) != 1 || s.ClaimLinks[0].FactIDs[0] != "f7" {
		t.Fatal("applicable symptom citation missing")
	}
	s = c.Explain(0, "nasal allergies", "allergic rhinitis", 34, map[string]string{"f7": "Abdominal pain"})
	if s.Relation != "related_reading" || len(s.ClaimLinks) != 0 {
		t.Fatal("unrelated finding was assigned a source-supported claim")
	}
	s = c.Explain(0, "nasal allergies", "allergic rhinitis", 7, map[string]string{"f7": "Sneezing"})
	if s.Relation != "related_reading" || len(s.ClaimLinks) > 0 {
		t.Fatal("out-of-population source linked to finding")
	}
	s = c.Explain(0, "unrepresented condition", "", 34, map[string]string{"f7": "Sneezing"})
	if s.Relation != "unavailable" {
		t.Fatal("symptom-only match supported unindexed condition")
	}
}
