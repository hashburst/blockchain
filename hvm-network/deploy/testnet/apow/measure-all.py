#!/usr/bin/env python3
"""Read-only network gates and isolated staged-miner calibration on four VPSs."""
import concurrent.futures,hashlib,importlib.util,json,math,tarfile,tempfile,time
from pathlib import Path
R=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('preflight',R/'preflight.py');pre=importlib.util.module_from_spec(spec);spec.loader.exec_module(pre)
def summarize(results):
 if len(results)!=4 or len({r['node_id'] for r in results})!=4 or len({r['address'].lower() for r in results})!=4:raise ValueError('four distinct miners required')
 rates=[r['measured_hashes_per_second'] for r in results]
 if any(not math.isfinite(x) or x<=0 for x in rates):raise ValueError('invalid hashrate')
 # Conservative first test: expected work <= one second on the slowest measured miner.
 bits=max(1,min(24,math.floor(math.log2(min(rates)))))
 return {'status':'measurement_only_not_activatable','chain_id':4735490,'proposed_initial_bits':bits,'proposed_min_bits':max(1,bits-4),'proposed_max_bits':min(24,bits+2),'proposed_window':32,'proposed_target_seconds':5,'activation_height':None,'gas_limit_change':None,'slowest_measured_hashes_per_second':min(rates),'timeouts':sum(s['status']=='timeout' for r in results for s in r['samples']),'limits':'Short synthetic sample under 25% CPU quota; not a liveness, fairness or reward guarantee. Retarget, proposer/network delays and competition require live acceptance.'}
def main():
 out=Path(tempfile.mkdtemp(prefix='apow-measure-results-',dir=R));sessions=[]
 try:
  source=pre.SOURCE+'\ninspect=main\n'+(R/'stage-node.py').read_text()+'\n'+(R/'measure-node.py').read_text()
  for host,node in pre.TARGETS:
   print('SSH_AUTH='+host,flush=True);sessions.append(pre.transport.Session(pre.transport.ssh_command(host),source))
  def rows():
   with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
    return list(pool.map(lambda x:x[0].call({'action':'status','node_id':x[1][1],'_timeout':45}),zip(sessions,pre.TARGETS)))
  first=rows();pre.validate_rows(first)
  if any(r['protocol'].get('apow') for r in first):raise ValueError('APoW already configured')
  height=min(r['height'] for r in first)
  proofs=[s.call({'action':'commitment','height':height,'_timeout':45}) for s in sessions];pre.compare(proofs,height)
  (out/'nodes.json').write_text(json.dumps(first,indent=2));(out/'commitments.json').write_text(json.dumps(proofs,indent=2))
  print('FIVE_NODE_AGREEMENT_OK height='+str(height),flush=True)
  results=[];errors=[]
  with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
   pending={pool.submit(s.call,{'action':'measure','node_id':node,'_timeout':300}):node for s,(_,node) in zip(sessions[:4],pre.TARGETS[:4])}
   while pending:
    done,_=concurrent.futures.wait(pending,timeout=10,return_when=concurrent.futures.FIRST_COMPLETED)
    if not done:print('MEASURING='+','.join(pending.values()),flush=True)
    for f in done:
     node=pending.pop(f)
     try:
      result=f.result();results.append(result);(out/(node+'.json')).write_text(json.dumps(result,indent=2)+'\n')
      print('MINER_MEASURED='+node+' hashes_per_second='+str(round(result['measured_hashes_per_second'],2)),flush=True)
     except Exception as e:errors.append(node+': '+str(e))
  if errors:raise ValueError('; '.join(errors))
  last=rows();pre.validate_rows(last)
  for a,b in zip(first,last):
   if b['height']<=a['height'] or b['pin_sha256']!=a['pin_sha256'] or b['binary_sha256']!=a['binary_sha256']:raise ValueError('finality/configuration gate failed')
  final_height=min(r['height'] for r in last)
  final_proofs=[s.call({'action':'commitment','height':final_height,'_timeout':45}) for s in sessions];pre.compare(final_proofs,final_height)
  (out/'progress.json').write_text(json.dumps(last,indent=2));(out/'final-commitments.json').write_text(json.dumps(final_proofs,indent=2))
  proposal=summarize(results);(out/'PARAMETERS_PROPOSAL.json').write_text(json.dumps(proposal,indent=2))
  print('FOUR_MINERS_MEASURED_NO_LIVE_WORK_SUBMITTED')
  print('FIVE_NODE_FINALITY_PRESERVED_NO_ACTIVATION')
  print(json.dumps(proposal,indent=2))
 except Exception as e:
  (out/'ERROR.txt').write_text(str(e));raise SystemExit('STOP: '+str(e)+'; node services/configuration retained')
 finally:
  for s in sessions:s.close()
  archive=Path(str(out)+'.tar.gz')
  with tarfile.open(archive,'w:gz') as t:t.add(out,arcname=out.name)
  print('REPORT_ARCHIVE='+str(archive),flush=True)
if __name__=='__main__':main()
