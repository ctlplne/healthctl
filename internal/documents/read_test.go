package documents

import (
	"context"
	"testing"
)

func TestSourceColumnsAndAmbiguousCollectionDates(t *testing.T) {
	_, rows, err := Read(context.Background(), "synthetic.txt", []byte("Collection date: 09/10/2026\nIssue date: 10/10/2026\nTest\tResult\tUnits\tReference range\nMarker\t<1.8\t\t1–3"))
	if err != nil {
		t.Fatal(err)
	}
	row := rows[len(rows)-1]
	if row.Value != "<1.8" || row.Units != "" || row.ReferenceRange != "1–3" || row.CollectionDate != "09/10/2026" || row.IssueDate != "10/10/2026" {
		t.Fatalf("source columns moved: %+v", row)
	}
	_, rows, err = Read(context.Background(), "synthetic.txt", []byte("Collected: May 1\nCollected: May 2\nMarker: 12.7 g/dL (ref 12–15) H"))
	if err != nil {
		t.Fatal(err)
	}
	row = rows[len(rows)-1]
	if row.Value != "12.7" || row.Units != "g/dL" || row.ReferenceRange != "12–15" || row.Flag != "H" || row.CollectionDate != "" {
		t.Fatalf("ambiguous result: %+v", row)
	}
}

func TestUnlabelledUnitOrFlagNeedsReview(t *testing.T) {
	_, rows, err := Read(context.Background(), "flag.txt", []byte("Marker: 1.2 L\nMarker: 2.1 HH"))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Units != "" || rows[0].Flag != "" || !rows[0].Uncertain || rows[0].Original != "Marker: 1.2 L" {
		t.Fatal("ambiguous token was assigned or lost")
	}
	if rows[1].Flag != "HH" || rows[1].Units != "" {
		t.Fatal("critical flag was moved into units")
	}
}
