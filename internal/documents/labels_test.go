package documents

import (
	"github.com/ctlplne/healthctl/internal/clinical"
	"testing"
)

func TestLabelCandidatesPreservePrintedStrength(t *testing.T) {
	for _, x := range []struct{ line, name, value, units string }{{"Example syrup 250 mg/5 mL take as directed", "Example syrup", "250", "mg/5 mL"}, {"Example cream 1%", "Example cream", "1", "%"}, {"Fictional tablets 0,5 mg", "Fictional tablets", "0,5", "mg"}, {"unreadable print", "", "", ""}} {
		rows := []clinical.ReportRow{{Original: x.line}}
		LabelCandidates(rows)
		r := rows[0]
		if r.Original != x.line || !r.Uncertain || r.Name != x.name || r.Value != x.value || r.Units != x.units {
			t.Errorf("incorrect label candidate: %+v", r)
		}
	}
}
