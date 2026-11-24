# Safety and extraction corpus

Synthetic sentences used as a software regression set. They are development content and have not been clinically reviewed. No real person or record appears here.

Each `.txt` file is named for its category. Lines starting with `#` are comments. Every other line has four fields separated by ` | `:

```
expect | source | text | extractions
```

- `expect` is `warn` when the app must show an urgent warning (either tier) and `none` when it must not.
- `source` is `self` (the user's own words), `witness` (someone describing the user), or `reply: <question>` (the user's answer to that question).
- `text` is what the person wrote.
- `extractions` is a `;`-separated list. Each item is `[!]status[/subject] "quote" concept [@ time]`.
  - `status` is `present`, `denied`, `uncertain` or `declined`.
  - `subject` is `self` (default), `family` or `other`.
  - `quote` must appear exactly once in the text.
  - A leading `!` marks an extraction the validator must reject.
  - `@ time` sets the finding's time context.

`go test -run TestCorpus -v ./internal/clinical` prints the metrics. The thresholds are 0 missed urgent sentences, at most 5% of correct extractions rejected, and 0 must-reject extractions accepted. Change a label only when it is genuinely wrong, and record the change in `HARDENING.md`.
