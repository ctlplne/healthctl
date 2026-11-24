// Package documents reads the writing on medical reports entirely on this installation.
package documents

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/ctlplne/healthctl/internal/sandbox"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ctlplne/healthctl/internal/clinical"
)

const MaxFile = 12 << 20
const maxPages = 24
const maxRows = 320
const maxLine = 1800

var ErrUnreadable = errors.New("No readable writing was found. Try a clearer copy or paste a transcription.")

type page struct {
	text, location string
	uncertain      bool
}

func Read(ctx context.Context, filename string, b []byte) (string, []clinical.ReportRow, error) {
	if len(b) == 0 || len(b) > MaxFile {
		return "", nil, errors.New("Choose a nonempty file up to 12 MiB.")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	var pages []page
	var err error
	switch ext {
	case ".txt":
		if !utf8.Valid(b) || bytes.ContainsRune(b, 0) {
			return "", nil, errors.New("Text files must contain UTF-8 text.")
		}
		pages = []page{{string(b), "Text", false}}
	case ".docx":
		pages, err = word(b)
	case ".pdf", ".jpg", ".jpeg", ".png":
		pages, err = readPages(ctx, ext, b)
	default:
		return "", nil, errors.New("Choose a PDF, DOCX, UTF-8 text file, JPEG or PNG.")
	}
	if err != nil {
		return "", nil, err
	}
	var rows []clinical.ReportRow
	for _, p := range pages {
		var header map[string]int
		collected, issued := uniqueDates(p.text)
		for n, line := range strings.Split(strings.ReplaceAll(p.text, "\r\n", "\n"), "\n") {
			original := strings.TrimSpace(line)
			if original == "" {
				continue
			}
			if len(original) > maxLine || len(rows) >= maxRows {
				return "", nil, errors.New("This document exceeds the review limit. Upload a shorter excerpt.")
			}
			row := clinical.ReportRow{ID: fmt.Sprintf("r%d", len(rows)+1), Original: original, Locator: fmt.Sprintf("%s, line %d", p.location, n+1), Uncertain: p.uncertain}
			if ext == ".docx" {
				line = strings.TrimSuffix(line, "\t")
			}
			cells := tableCells(line)
			if mapping := headings(cells); mapping != nil {
				header = mapping
			} else if header != nil && len(cells) == len(header) {
				get := func(name string) string {
					if i, ok := header[name]; ok {
						return strings.TrimSpace(cells[i])
					}
					return ""
				}
				row.Name = get("name")
				row.Value = get("value")
				row.Units = get("units")
				row.ReferenceRange = get("range")
				row.Flag = get("flag")
				row.CollectionDate = get("collection")
				row.IssueDate = get("issued")
			} else if candidate := labeledResult.FindStringSubmatch(original); candidate != nil {
				row.Name = candidate[1]
				row.Value = candidate[2]
				row.Units = candidate[3]
				row.ReferenceRange = candidate[4]
				row.Flag = candidate[5]
				if row.Flag == "" {
					switch strings.ToUpper(row.Units) {
					case "HH", "LL", "CRITICAL":
						row.Flag, row.Units = row.Units, ""
					case "H", "L":
						// An unlabelled token may be a unit or a laboratory flag.
						// Keep the printed line and request review before assigning it.
						row.Units = ""
						row.Uncertain = true
					}
				}
			}
			if row.Name != "" {
				if row.CollectionDate == "" {
					row.CollectionDate = collected
				}
				if row.IssueDate == "" {
					row.IssueDate = issued
				}
			}
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		if ext == ".txt" {
			return "", nil, ErrUnreadable
		}
		// Keep the original so the owner can transcribe a visible page/region.
		rows = []clinical.ReportRow{}
	}
	return strings.TrimPrefix(ext, "."), rows, nil
}

var spaces = regexp.MustCompile(` {2,}`)
var labeledResult = regexp.MustCompile(`(?i)^([a-z][a-z0-9 /_()-]{1,65}):[ \t]*([<>≤≥]?[ ]*[-+]?[0-9]+(?:[.,][0-9]+)?|trace|positive|negative|present|absent|detected|not detected)(?:[ ]+([a-zA-Zµμ%][a-zA-Zµμ0-9%/^.-]{0,24}))?(?:[ ]*\((?:ref(?:erence)?[.: ]+)([^()]+)\))?(?:[ ]+(H|L|HH|LL|critical))?$`)
var labeledDate = regexp.MustCompile(`(?im)^\s*(collection date|specimen collected|collected|issue date|report issued|reported on):[ \t]*([^\r\n]{1,45})[ \t]*$`)

func uniqueDates(text string) (string, string) {
	var collected, issued []string
	for _, m := range labeledDate.FindAllStringSubmatch(text, -1) {
		switch strings.ToLower(m[1]) {
		case "collection date", "specimen collected", "collected":
			collected = append(collected, strings.TrimSpace(m[2]))
		default:
			issued = append(issued, strings.TrimSpace(m[2]))
		}
	}
	one := func(d []string) string {
		if len(d) == 1 {
			return d[0]
		}
		return ""
	}
	return one(collected), one(issued)
}

func tableCells(s string) []string {
	s = strings.Trim(s, " \r\f")
	var parts []string
	switch {
	case strings.Contains(s, "|"):
		parts = strings.Split(strings.Trim(s, "|"), "|")
	case strings.Contains(s, "\t"):
		parts = strings.Split(s, "\t")
	default:
		parts = spaces.Split(s, -1)
	}
	return parts
}

func headings(cells []string) map[string]int {
	keys := map[string]string{"test": "name", "test name": "name", "analyte": "name", "investigation": "name", "result": "value", "value": "value", "unit": "units", "units": "units", "range": "range", "reference": "range", "reference range": "range", "reference interval": "range", "flag": "flag", "collected": "collection", "collection date": "collection", "issued": "issued", "issue date": "issued"}
	m := map[string]int{}
	for i, c := range cells {
		k := keys[strings.ToLower(strings.TrimSpace(c))]
		if k == "" {
			return nil
		}
		if _, ok := m[k]; ok {
			return nil
		}
		m[k] = i
	}
	_, n := m["name"]
	_, v := m["value"]
	if !n || !v {
		return nil
	}
	return m
}

func word(b []byte) ([]page, error) {
	archive, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil || len(archive.File) > 1500 {
		return nil, errors.New("The Word file could not be read.")
	}
	for _, f := range archive.File {
		if f.Name != "word/document.xml" {
			continue
		}
		if f.UncompressedSize64 > 4<<20 {
			return nil, errors.New("This Word document exceeds the text limit.")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, ErrUnreadable
		}
		defer rc.Close()
		d := xml.NewDecoder(io.LimitReader(rc, 4<<20))
		var out strings.Builder
		inText, cell := false, false
		for {
			tok, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, ErrUnreadable
			}
			switch v := tok.(type) {
			case xml.StartElement:
				switch v.Name.Local {
				case "t":
					inText = true
				case "tc":
					cell = true
				case "tab":
					out.WriteByte('\t')
				case "br":
					out.WriteByte('\n')
				}
			case xml.CharData:
				if inText {
					out.Write(v)
				}
			case xml.EndElement:
				switch v.Name.Local {
				case "t":
					inText = false
				case "p":
					if cell {
						out.WriteByte(' ')
					} else {
						out.WriteByte('\n')
					}
				case "tc":
					cell = false
					out.WriteByte('\t')
				case "tr":
					out.WriteByte('\n')
				}
			}
		}
		return []page{{out.String(), "Word document", false}}, nil
	}
	return nil, ErrUnreadable
}

type bounded struct {
	bytes.Buffer
	limit int
}

func (b *bounded) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("extraction output limit")
	}
	return b.Buffer.Write(p)
}
func run(ctx context.Context, name string, args ...string) (string, error) {
	// Native readers receive no environment secrets, URLs, or caller-controlled
	// switches, and no network where the system allows that.
	cmd, cancel := sandbox.Command(ctx, sandbox.DocumentLimits, name, args...)
	defer cancel()
	var out bounded
	out.limit = 1 << 20
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", errors.New("Local document reader could not read this file. Use a shorter excerpt or paste text instead.")
	}
	return out.String(), nil
}
func readPages(ctx context.Context, ext string, b []byte) ([]page, error) {
	dir, err := os.MkdirTemp("", "clinical-read-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	src := filepath.Join(dir, "source"+ext)
	if err = os.WriteFile(src, b, 0600); err != nil {
		return nil, err
	}
	if ext != ".pdf" {
		info, format, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil || (format != "png" && format != "jpeg") || int64(info.Width)*int64(info.Height) > 24000000 {
			return nil, errors.New("Choose a readable JPEG or PNG of up to 24 megapixels.")
		}
		line, err := run(ctx, "tesseract", src, "stdout", "-l", "eng", "--psm", "6")
		return []page{{line, "Image (OCR)", true}}, err
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return nil, ErrUnreadable
	}
	info, err := run(ctx, "pdfinfo", src)
	if err != nil {
		return nil, err
	}
	match := regexp.MustCompile(`(?m)^Pages:\s+(\d+)\s*$`).FindStringSubmatch(info)
	if len(match) != 2 {
		return nil, ErrUnreadable
	}
	count, _ := strconv.Atoi(match[1])
	if count < 1 || count > maxPages {
		return nil, errors.New("Choose a PDF of 1–24 pages.")
	}
	listed, err := run(ctx, "pdfimages", "-list", src)
	if err != nil {
		return nil, err
	}
	visual := map[int]bool{}
	for _, line := range strings.Split(listed, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[2] != "image" {
			continue
		}
		pn, _ := strconv.Atoi(fields[0])
		width, _ := strconv.Atoi(fields[3])
		height, _ := strconv.Atoi(fields[4])
		if width >= 500 && height >= 300 || width >= 300 && height >= 500 {
			visual[pn] = true
		}
	}
	var pages []page
	for i := 1; i <= count; i++ {
		n := strconv.Itoa(i)
		label := "Page " + n
		t, err := run(ctx, "pdftotext", "-f", n, "-l", n, "-layout", "-enc", "UTF-8", src, "-")
		if err != nil {
			return nil, err
		}
		// OCR a scanned page even in a document that also contains digital pages.
		if strings.TrimSpace(t) != "" {
			pages = append(pages, page{t, label, false})
			if !visual[i] {
				continue
			}
		}
		rendered := filepath.Join(dir, "page")
		if _, err = run(ctx, "pdftoppm", "-f", n, "-l", n, "-singlefile", "-scale-to", "2200", "-png", src, rendered); err != nil {
			return nil, err
		}
		digital := t
		t, err = run(ctx, "tesseract", rendered+".png", "stdout", "-l", "eng", "--psm", "6")
		if err != nil {
			return nil, err
		}
		// On a mixed page, retain digital rows and add only different OCR lines.
		existing := map[string]bool{}
		for _, line := range strings.Split(digital, "\n") {
			existing[strings.Join(strings.Fields(line), " ")] = true
		}
		var extra []string
		for _, line := range strings.Split(t, "\n") {
			if !existing[strings.Join(strings.Fields(line), " ")] {
				extra = append(extra, line)
			}
		}
		pages = append(pages, page{strings.Join(extra, "\n"), label + " (OCR)", true})
	}
	return pages, nil
}

// Preview renders the original page for visual comparison with the transcription.
// Never embed an original PDF or document's active content in the owner interface.
func Preview(ctx context.Context, kind string, data []byte, number int) ([]byte, error) {
	if number < 1 || number > maxPages {
		return nil, errors.New("Choose a page in the document.")
	}
	if kind == "pdf" {
		dir, err := os.MkdirTemp("", "clinical-preview-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		src := filepath.Join(dir, "source.pdf")
		if err = os.WriteFile(src, data, 0600); err != nil {
			return nil, err
		}
		n := strconv.Itoa(number)
		out := filepath.Join(dir, "page")
		if _, err = run(ctx, "pdftoppm", "-f", n, "-l", n, "-singlefile", "-scale-to", "1650", "-png", src, out); err != nil {
			return nil, err
		}
		return os.ReadFile(out + ".png")
	}
	if (kind != "jpeg" && kind != "jpg" && kind != "png") || number != 1 {
		return nil, errors.New("This format has a text preview and an original download.")
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || int64(info.Width)*int64(info.Height) > 24000000 {
		return nil, ErrUnreadable
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnreadable
	}
	width, height := info.Width, info.Height
	const limit = 1650
	if width >= height && width > limit {
		height = max(1, height*limit/width)
		width = limit
	} else if height > limit {
		width = max(1, width*limit/height)
		height = limit
	}
	view := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < width; x++ {
			view.Set(x, y, source.At(bounds.Min.X+x*info.Width/width, bounds.Min.Y+y*info.Height/height))
		}
	}
	var result bytes.Buffer
	if err = png.Encode(&result, view); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

// LabelCandidates preserves the entire printed line. Parsing a strength does
// not establish a prescription, an equivalent product or the patient's dose.
var labelStrength = regexp.MustCompile(`(?i)^(.{1,130}?)\s+([0-9]+(?:[.,][0-9]+)?)\s*((?:mcg|mg|g|mL|units?|IU|%)(?:\s*/\s*(?:[0-9]+(?:[.,][0-9]+)?\s*)?(?:mL|L|g|tablet|dose))?)(?:$|[^[:alnum:]])`)

func LabelCandidates(rows []clinical.ReportRow) {
	for i := range rows {
		r := &rows[i]
		m := labelStrength.FindStringSubmatch(r.Original)
		if m != nil {
			r.Name = strings.TrimSpace(m[1])
			r.Value = m[2]
			r.Units = m[3]
		}
		r.Uncertain = true
	}
}
