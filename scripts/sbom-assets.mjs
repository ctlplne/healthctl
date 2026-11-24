// Record the shipped assets that a binary and OS package scan cannot identify.
import fs from 'node:fs'
import crypto from 'node:crypto'

const [input, output, root, lockPath] = process.argv.slice(2)
const sbom = JSON.parse(fs.readFileSync(input))
const lock = JSON.parse(fs.readFileSync(lockPath)).packages
const digest = (file) => crypto.createHash('sha256').update(fs.readFileSync(root + file)).digest('hex')
const owner = sbom.relationships.find((r) => r.spdxElementId === 'SPDXRef-DOCUMENT' && r.relationshipType === 'DESCRIBES')?.relatedSpdxElement
if (!owner) throw Error('Missing root SPDX package')
const components = new Set(sbom.packages.map((p) => p.name))
for (const name of ['poppler-utils', 'tesseract-ocr', 'tesseract-ocr-eng', 'github.com/jackc/pgx/v5']) {
  if (!components.has(name)) throw Error('Runtime SBOM is missing ' + name)
}
function add(name, versionInfo, downloadLocation, license, checksum) {
  const id = 'SPDXRef-HealthctlAsset-' + name.replace(/[^a-zA-Z0-9.-]/g, '-')
  sbom.packages.push({name, SPDXID: id, versionInfo, downloadLocation, licenseDeclared: license,
    licenseConcluded: 'NOASSERTION', filesAnalyzed: false, copyrightText: 'NOASSERTION',
    ...(checksum ? {checksums: [{algorithm: 'SHA256', checksumValue: checksum}]} : {})})
  sbom.relationships.push({spdxElementId: owner, relationshipType: 'CONTAINS', relatedSpdxElement: id})
}
for (const [path, pkg] of Object.entries(lock)) {
  if (!path || pkg.dev) continue
  const name = path.slice(path.lastIndexOf('node_modules/') + 'node_modules/'.length)
  add(name, pkg.version, pkg.resolved, pkg.license || 'NOASSERTION')
}
add('whisper.cpp', '1.9.5', 'https://github.com/ggml-org/whisper.cpp/tree/v1.9.5', 'MIT', digest('/usr/local/bin/whisper-cli'))
add('ggml-base.en-q5_1.bin', 'base.en-q5_1', 'https://huggingface.co/ggerganov/whisper.cpp', 'MIT', digest('/usr/share/healthctl/whisper.bin'))
const path = '/usr/share/licenses/clinical-history/starter.json'
const evidence = JSON.parse(fs.readFileSync(root + path))
add(evidence.id, evidence.version, 'https://github.com/ctlplne/healthctl', 'NOASSERTION', digest(path))
fs.writeFileSync(output, JSON.stringify(sbom) + '\n')
