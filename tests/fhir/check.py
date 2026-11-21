"""Validate a serialized clinical-history document with the HL7 R4 JSON Schema."""

import hashlib
import json
from pathlib import Path
import sys
import zipfile
from xml.etree import ElementTree

from jsonschema import Draft6Validator

archive = Path(__file__).with_name("r4-schema.zip")
assert (
    hashlib.sha256(archive.read_bytes()).hexdigest()
    == "75e5560da3cf503895a44c8ca7af17a83b4cca6c2cb5ba1883d2aec0d1cb5ac6"
)
with zipfile.ZipFile(archive) as package:
    core = json.loads(package.read("fhir.schema.json"))
bundle = json.load(sys.stdin)
# Select the Bundle branch directly; its entries still go through HL7's complete
# ResourceList union. All references in the official schema resolve locally.
schema = {"$ref": "#/definitions/Bundle", "definitions": core["definitions"]}
Draft6Validator(schema).validate(bundle)
assert (
    bundle["type"] == "document"
    and bundle["timestamp"]
    and bundle["identifier"]["system"]
)
entries = bundle["entry"]
by_url = {e["fullUrl"]: e["resource"] for e in entries}
assert (
    len(by_url) == len(entries)
    and entries[0]["resource"]["resourceType"] == "Composition"
)
composition = entries[0]["resource"]
assert by_url[composition["subject"]["reference"]]["resourceType"] == "Patient"
assert by_url[composition["author"][0]["reference"]]["resourceType"] == "Device"
for url, resource in by_url.items():
    assert url == "urn:uuid:" + resource["id"]
    assert resource["meta"]["profile"] == [
        "http://hl7.org/fhir/StructureDefinition/" + resource["resourceType"]
    ]


def check(value):
    if isinstance(value, list):
        for item in value:
            check(item)
    elif isinstance(value, dict):
        if "reference" in value:
            assert value["reference"] in by_url
        if "div" in value:
            assert (
                ElementTree.fromstring(value["div"]).tag
                == "{http://www.w3.org/1999/xhtml}div"
            )
        for item in value.values():
            check(item)


check(bundle)
assert any(r["resourceType"] == "OperationOutcome" for r in by_url.values())
print(
    "PASS HL7 R4 4.0.1 JSON Schema, document composition, named base profiles, references, narratives and mapping-loss report."
)
