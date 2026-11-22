"""Exercise a Windows-built executable, natively and against real Linux containers."""
import hashlib
import json
import os
from pathlib import Path
import secrets
import struct
import subprocess
import sys
import tarfile
import tempfile
from urllib.parse import urlsplit
from verify_restore import Client, demo

root = Path(__file__).resolve().parents[1]
out = root/'build/windows'
exe = out/'healthctl-windows-x64.exe'
native = os.name == 'nt'
command = [str(exe)] if native else ['mono', str(exe)]
digest = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
with tempfile.TemporaryDirectory(prefix='healthctl win test ') as temp:
    temp = Path(temp)
    state = temp/'private workspace'
    env = dict(os.environ, HEALTHCTL_DESKTOP_STATE=str(state))
    def run(*args, **kwargs):
        return subprocess.run([*command, *map(str,args)], env=env, check=True, text=True, **kwargs)
    binary = exe.read_bytes()
    header = struct.unpack_from('<I',binary,0x3c)[0]
    assert binary[:2] == b'MZ' and binary[header:header+4] == b'PE\0\0' and struct.unpack_from('<H',binary,header+4)[0] == 0x8664
    for line in (out/'SHA256SUMS').read_text().splitlines():
        value, name = line.split('  ')
        assert digest(out/name) == value
    assets = temp/'unpacked'
    run('--extract', assets)
    for source, bundled in [('internal/clinical/preview.txt', 'preview.txt'), ('docs/use-notice.md', 'use-notice.md'), ('LICENSE', 'LICENSE')]:
        assert (assets/bundled).read_bytes() == subprocess.check_output(['git','show','HEAD:'+source],cwd=root), bundled
    rev, db, source_sha = (assets/'release.txt').read_text().splitlines()
    assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()==rev
    assert digest(assets/'source.tar.gz')==source_sha
    inventory='healthctl-windows-x64.spdx.json'
    assert (assets/inventory).read_bytes()==(out/inventory).read_bytes()
    assert json.loads((assets/inventory).read_bytes())['packages'][1]['checksums'][0]['checksumValue']==source_sha
    with tarfile.open(assets/'source.tar.gz') as tar:
        for member in tar.getmembers():
            if member.isfile():
                blob=subprocess.check_output(['git','rev-parse',rev+':'+member.name],cwd=root,text=True).strip()
                assert hashlib.sha1(b'blob '+str(member.size).encode()+b'\0'+tar.extractfile(member).read()).hexdigest()==blob
    if native:
        # Start the same WinForms entry point used by Explorer, find its real window and close it normally.
        subprocess.run(['powershell','-NoProfile','-NonInteractive','-Command',
                        "$p=Start-Process -FilePath $env:HEALTHCTL_VERIFY_EXE -PassThru; try { $null=$p.WaitForInputIdle(15000); $end=(Get-Date).AddSeconds(15); do { Start-Sleep -Milliseconds 150; $p.Refresh() } while (!$p.HasExited -and !$p.MainWindowHandle -and (Get-Date) -lt $end); if ($p.MainWindowTitle -notlike 'healthctl*') { throw 'Launcher window missing' }; if (!$p.CloseMainWindow() -or !$p.WaitForExit(10000)) { throw 'Launcher did not close normally' } } finally { if (!$p.HasExited) { $p.Kill() } }"],
                       env=dict(env, HEALTHCTL_VERIFY_EXE=str(exe)),check=True)
        print('Verified x64 Windows GUI launch and close, checksums, source integrity, and embedded notices.')
        sys.exit()
    def start():
        url=run('--start', stdout=subprocess.PIPE, timeout=2200).stdout.strip()
        parsed=urlsplit(url)
        assert parsed.scheme=='http' and parsed.hostname=='127.0.0.1' and parsed.port
        return url,Client(str(parsed.port))
    try:
        addr,c=start()
        owner={'username':'windows-fictional','password':secrets.token_urlsafe(24)}
        info=c.call('/setup','POST',owner)
        c.call('/settings','PUT',{'provider':demo})
        enc=c.call('/encounters','POST',{'age':38,'concern':'Things sound different when I hum.','demoPractice':True,
            'guidedHistory':True,'narrativeFirst':True,'pregnancyRelated':False,'urgentNow':False,
            'noticeVersion':info['noticeVersion'],'requestId':secrets.token_hex(12),'disclosedProvider':demo})
        path='/encounters/'+enc['id']
        assert enc['question'].startswith('When did you first')
        answer='Last week while practicing a song.'
        e=c.call(path+'/actions','POST',{'action':'answer','text':answer,'revision':enc['revision'],'requestId':secrets.token_hex(12)})
        assert e['question'].startswith('What has changed')
        original=(state/'workspace.conf').read_bytes()
        assert start()[0]==addr and (state/'workspace.conf').read_bytes()==original
        assert any(a['text']==answer for a in c.call(path+'/export')['answers'])
        print('Verified first build, loopback startup, owner, open-ended interview and exported answer after restart.')
    finally:
        settings=state/'workspace.conf'
        if settings.exists():
            project,port,key,password=settings.read_text().splitlines()
            subprocess.run(['docker','compose','-p',project,'-f',str(assets/'compose.yaml'),'-f',str(assets/'desktop.yaml'),'down','-v'],
                env=dict(env,CLINICAL_PORT=port,CLINICAL_DATA_KEY=key,POSTGRES_PASSWORD=password,
                         HEALTHCTL_APP_IMAGE='healthctl-local/app:'+rev,HEALTHCTL_DB_IMAGE=db),check=True)
