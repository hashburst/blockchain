from pathlib import Path
import json,subprocess,tempfile,shutil,hashlib
p=Path(__file__).resolve().parent
with tempfile.TemporaryDirectory() as tmp:
 d=Path(tmp);key=d/'p2p.key'
 peer=subprocess.check_output([p/'observer-identity','--key',key],text=True).strip()
 old=key.read_bytes()
 again=subprocess.run([p/'observer-identity','--key',key],capture_output=True)
 assert again.returncode!=0 and key.read_bytes()==old
 c=json.loads((p/'template.json').read_text());c.update(role='observer',node_id='hvm-testnet-ingress',peer_id=peer,p2p_key_file=str(key),data_dir=str(d/'state'),validator_id='',consensus_key_file='')
 (d/'state').mkdir(mode=0o700)
 for f in (p/'checkpoint').iterdir():shutil.copyfile(f,d/'state'/f.name)
 config=d/'node.json';config.write_text(json.dumps(c))
 for arg in ['--provision','--check']:
  subprocess.run([p/'hashburst-testnet','--config',config,arg],check=True)
 assert all((d/'state'/n).stat().st_size==0 for n in ['consensus-votes.jsonl','consensus-bft-signatures.jsonl'])
 c['consensus_key_file']=str(d/'forbidden.key');config.write_text(json.dumps(c))
 bad=subprocess.run([p/'hashburst-testnet','--config',config,'--check'],capture_output=True,text=True)
 assert bad.returncode and 'observer must not load signing key' in bad.stderr
 print('OBSERVER_OFFLINE_TESTS_OK: exclusive P2P key, existing checkpoint provision/check, empty signing journals, signing-key configuration rejected')
