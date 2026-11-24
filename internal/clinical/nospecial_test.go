package clinical

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestNoSpecialCaseRegexes keeps context rules in the engine. A regular
// expression outside context.go may classify content (a symptom, a report row,
// advice), but it may not decide negation, who an experience belongs to, time or
// hypotheticals. Those decisions belong to context.go, where the corpus tests them.
func TestNoSpecialCaseRegexes(t *testing.T) {
	cues := regexp.MustCompile(`(?i)\(\?:?(?:my\|our|my\|his)|\b(?:father|mother|grandmother)\b.*\b(?:friend|child)\b|aren\['’\]t|weren\['’\]t|wasn\['’\]t|isn\['’\]t|\bno longer\b|\bpreviously\b|in the past|history of|\bunless\b|in case of|should you`)
	// Content classifiers that mention such words for their own purpose.
	allowed := map[string]string{
		"alwaysBlocked": "advice and certainty language in model output",
		"disagreement":  "a user disputing a report row",
		"pastEventCue":  "a dated past event, used with the engine's history scope",
	}
	files, _ := os.ReadDir(".")
	fset := gotoken.NewFileSet()
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "context.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, v := range spec.Values {
				call, ok := v.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					continue
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "MustCompile" {
					continue
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok {
					continue
				}
				expr, _ := strconv.Unquote(lit.Value)
				id := spec.Names[i].Name
				if cues.MatchString(expr) && allowed[id] == "" {
					t.Errorf("%s: %s decides context outside the engine; move it into context.go", name, id)
				}
			}
			return true
		})
	}
	for _, id := range []string{"thirdParty", "pureDenial", "bareDenial", "historicalQuestion", "eyeDenial", "conditionalPrinted", "documentCurrent"} {
		src, _ := os.ReadFile("context.go")
		if !strings.Contains(string(src), "var "+id+" = ") {
			t.Errorf("%s must be defined in the context engine", id)
		}
	}
}
