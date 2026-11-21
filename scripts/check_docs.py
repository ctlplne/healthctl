"""Check repository-relative Markdown links and heading fragments in tracked documents."""
from pathlib import Path
import re
import subprocess
from urllib.parse import unquote, urlsplit


def headings(document):
    found = set()
    for line in re.findall(r"^#{1,6}\s+(.+)$", document, re.MULTILINE):
        base = "".join(c for c in line.lower() if c.isalnum() or c in "-_ ").replace(" ", "-")
        anchor = base
        suffix = 0
        while anchor in found:
            suffix += 1
            anchor = f"{base}-{suffix}"
        found.add(anchor)
    return found


def main():
    root = Path.cwd()
    tracked = set(subprocess.check_output(["git", "ls-files", "-z"]).decode().split("\0")[:-1])
    docs = {root / p: (root / p).read_text() for p in tracked if p.endswith(".md")}
    anchors = {path: headings(source) for path, source in docs.items()}
    errors = []
    count = 0
    for path, source in docs.items():
        for number, line in enumerate(source.splitlines(), 1):
            for link in re.findall(r"\]\(([^)]+)\)", line):
                url = urlsplit(link.strip("<>"))
                if url.netloc or url.scheme:
                    continue
                count += 1
                target = (path.parent / unquote(url.path)).resolve() if url.path else path
                if (not target.is_relative_to(root) or target.relative_to(root).as_posix() not in tracked
                        or (target in docs and url.fragment and unquote(url.fragment) not in anchors[target])):
                    errors.append(f"{path.relative_to(root)}:{number}: unresolved link {link}")
    for error in sorted(errors):
        print(error)
    print(f"Checked {count} relative links in {len(docs)} documents.")
    return bool(errors)


if __name__ == "__main__":
    raise SystemExit(main())
