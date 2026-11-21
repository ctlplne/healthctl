"""Write an SPDX 2.3 index for the native launcher, backup helper and embedded images."""
import datetime as dt
import hashlib
import json
import plistlib
import subprocess
import sys
import uuid
from pathlib import Path

app = Path(sys.argv[1])
resources = app / "Contents/Resources"
notices = resources / "notices"
version = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleShortVersionString"]
packages = []
relations = []
external = []


def package(identifier, name, version, location="NOASSERTION", license="NOASSERTION"):
    packages.append(dict(SPDXID=identifier, name=name, versionInfo=version, downloadLocation=location,
                         filesAnalyzed=False, licenseDeclared=license, licenseConcluded="NOASSERTION",
                         copyrightText="NOASSERTION"))


def relation(source, kind, target):
    relations.append(dict(spdxElementId=source, relationshipType=kind, relatedSpdxElement=target))


root = "SPDXRef-healthctl-desktop"
package(root, "healthctl macOS launcher", version, "https://github.com/ctlplne/healthctl", "BUSL-1.1")
relation("SPDXRef-DOCUMENT", "DESCRIBES", root)
archive = "SPDXRef-backup"
package(archive, "healthctl-archive", version, "https://github.com/ctlplne/healthctl", "BUSL-1.1")
relation(root, "CONTAINS", archive)

lines = subprocess.check_output(["go", "version", "-m", str(resources / "archive")], text=True).splitlines()
package("SPDXRef-Go", "Go runtime", lines[0].split()[-1], "https://go.dev/dl/", "BSD-3-Clause")
relation(archive, "DEPENDS_ON", "SPDXRef-Go")
for line in lines[1:]:
    fields = line.split()
    if fields and fields[0] == "dep":
        module, ver = fields[1:3]
        identifier = "SPDXRef-go-" + str(len(packages))
        package(identifier, module, ver, "https://proxy.golang.org/" + module + "/@v/" + ver + ".zip")
        relation(archive, "DEPENDS_ON", identifier)

linked = subprocess.check_output(["otool", "-L", str(app / "Contents/MacOS/healthctl")], text=True)
for line in linked.splitlines()[1:]:
    path, _, detail = line.strip().partition(" (compatibility version ")
    current = detail.partition("current version ")[2].split(")")[0]
    identifier = "SPDXRef-macOS-" + str(len(packages))
    package(identifier, path, current or "NOASSERTION")
    relation(root, "DEPENDS_ON", identifier)

source = resources / "healthctl-source.tar.gz"
if source.is_file():
    revision = (resources / "images.txt").read_text().split()[0].split(":")[1]
    package("SPDXRef-source", "healthctl build source", revision,
            "https://github.com/ctlplne/healthctl/tree/" + revision, "BUSL-1.1")
    packages[-1]["checksums"] = [dict(algorithm="SHA256", checksumValue=hashlib.sha256(source.read_bytes()).hexdigest())]
    relation(root, "CONTAINS", "SPDXRef-source")

for name in (() if source.is_file() else ("app", "db")):
    data = (notices / (name + ".spdx.json")).read_bytes()
    child = json.loads(data)
    doc = "DocumentRef-" + name
    described = next(r["relatedSpdxElement"] for r in child["relationships"]
                     if r["spdxElementId"] == "SPDXRef-DOCUMENT" and r["relationshipType"] == "DESCRIBES")
    external.append(dict(externalDocumentId=doc, spdxDocument=child["documentNamespace"],
                         checksum=dict(algorithm="SHA256", checksumValue=hashlib.sha256(data).hexdigest())))
    relation(root, "CONTAINS", doc + ":" + described)

sbom = dict(spdxVersion="SPDX-2.3", dataLicense="CC0-1.0", SPDXID="SPDXRef-DOCUMENT",
            name="healthctl-macOS-" + version, documentNamespace="https://spdx.org/spdxdocs/healthctl-" + str(uuid.uuid4()),
            creationInfo=dict(created=dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                              creators=["Tool: healthctl-desktop-sbom"]), packages=packages,
            relationships=relations)
if external:
    sbom["externalDocumentRefs"] = external
(notices / "desktop.spdx.json").write_text(json.dumps(sbom, indent=2) + "\n")
