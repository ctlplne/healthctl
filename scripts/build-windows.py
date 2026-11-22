"""Build the self-contained x64 Windows launcher from the checked-out commit."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import uuid

root = Path(__file__).resolve().parents[1]
out = root / 'build/windows'
out.mkdir(parents=True, exist_ok=True)
revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
inputs = ['Dockerfile', '.dockerignore', 'LICENSE', 'PRIVACY.md', 'THIRD_PARTY_NOTICES.md',
          'go.mod', 'go.sum', 'cmd', 'internal', 'evaluation', 'web', 'docs/release.md',
          'docs/licenses/whisper-model.txt', 'scripts/go-notices.sh', 'scripts/sbom-assets.mjs']
name = 'healthctl-windows-x64'
with tempfile.TemporaryDirectory() as temp:
    work = Path(temp)
    def tracked(path):
        return subprocess.check_output(['git', 'show', revision + ':' + path], cwd=root)
    source = work / 'source.tar.gz'
    subprocess.run(['git', '-c', 'core.autocrlf=false', 'archive', '--format=tar.gz', '-o', str(source), revision, '--', *inputs], cwd=root, check=True)
    for path in ('compose.yaml', 'desktop/desktop.yaml', 'LICENSE', 'THIRD_PARTY_NOTICES.md', 'desktop/windows/Launcher.cs'):
        (work / Path(path).name).write_bytes(tracked(path))
    db = re.search(r'^\s+image: (postgres:[^\s]+@sha256:[a-f0-9]{64})$', (work/'compose.yaml').read_text(), re.M)[1]
    (work/'release.txt').write_text(f'{revision}\n{db}\n{sha(source)}\n', encoding='ascii')
    inventory = name + '.spdx.json'
    url = 'https://github.com/ctlplne/healthctl'
    def component(identifier, title, version, location, license):
        return dict(SPDXID=identifier, name=title, versionInfo=version, downloadLocation=location,
                    filesAnalyzed=False, licenseConcluded='NOASSERTION', licenseDeclared=license,
                    copyrightText='NOASSERTION')
    app = component('SPDXRef-Launcher', 'healthctl Windows x64 launcher', revision, url, 'BUSL-1.1')
    bundle = component('SPDXRef-Source', 'Bundled healthctl build source', revision, url+'/tree/'+revision, 'BUSL-1.1')
    bundle['checksums'] = [dict(algorithm='SHA256', checksumValue=sha(source))]
    sbom = dict(spdxVersion='SPDX-2.3', dataLicense='CC0-1.0', SPDXID='SPDXRef-DOCUMENT', name=name,
                documentNamespace=url+'/spdx/'+str(uuid.uuid4()),
                creationInfo=dict(creators=['Tool: healthctl Windows packager'], created=datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')),
                packages=[app, bundle], relationships=[
                    dict(spdxElementId='SPDXRef-DOCUMENT', relationshipType='DESCRIBES', relatedSpdxElement='SPDXRef-Launcher'),
                    dict(spdxElementId='SPDXRef-Launcher', relationshipType='CONTAINS', relatedSpdxElement='SPDXRef-Source')])
    (work/inventory).write_text(json.dumps(sbom,indent=2)+'\n',encoding='utf8')
    resources = ['source.tar.gz', 'compose.yaml', 'desktop.yaml', 'release.txt', 'LICENSE', 'THIRD_PARTY_NOTICES.md', inventory]
    compiler = Path(os.environ['WINDIR'])/'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
    executable = out / (name+'.exe')
    subprocess.run([str(compiler), '/nologo', '/target:winexe', '/platform:x64', '/optimize+',
                    '/reference:System.Windows.Forms.dll', '/reference:System.Drawing.dll', '/out:'+str(executable),
                    *['/resource:'+path+','+path for path in resources], 'Launcher.cs'], cwd=work, check=True)
    shutil.copyfile(work/inventory, out/inventory)
(out/'SHA256SUMS').write_bytes(''.join(sha(out/n)+'  '+n+'\n' for n in [executable.name, inventory]).encode('ascii'))
print('Built Windows x64 launcher at '+revision)
