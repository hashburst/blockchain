#!/usr/bin/env python3
import json,socket,threading,urllib.request
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
ALLOWED={'eth_chainId','net_version','eth_blockNumber','web3_clientVersion','eth_getBalance','eth_getTransactionCount','hb_getTransactionCount','hb_getTransactionReceipt','hb_getFinalizedHeight','hb_getFinalizedCommitment','hb_getHVMStateRoot','hb_getHBTStateRoot','hb_getStandards'}
MAX_BODY=65536
SLOTS=threading.BoundedSemaphore(16)
def validate(d):
 if not isinstance(d,dict) or d.get('jsonrpc')!='2.0' or type(d.get('id')) not in (str,int) or not isinstance(d.get('params',[]),list):raise ValueError('invalid request; batches and notifications are unsupported')
 if len(str(d['id']))>128:raise ValueError('id too large')
 if d.get('method') not in ALLOWED:raise PermissionError('method not available on public ingress')
 return {k:d[k] for k in ('jsonrpc','id','method','params') if k in d}
def upstream(path,body=None):
 req=urllib.request.Request('http://127.0.0.1:18009'+path,body,{'Content-Type':'application/json'})
 with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req,timeout=12) as r:
  raw=r.read(1048577)
  if len(raw)>1048576:raise ValueError('upstream response too large')
  return json.loads(raw)
class Handler(BaseHTTPRequestHandler):
 def setup(self):super().setup();self.connection.settimeout(15)
 def log_message(self,*args):pass
 def reply(self,status,d):
  raw=json.dumps(d,separators=(',',':')).encode();self.send_response(status)
  self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(raw)));self.send_header('Cache-Control','no-store');self.end_headers();self.wfile.write(raw)
 def do_GET(self):
  if self.path.split('?',1)[0]!='/health':return self.reply(404,{'error':'not_found'})
  if not SLOTS.acquire(False):return self.reply(503,{'error':'busy'})
  try:
   d=upstream('/health')
   if d.get('chain_id')!=4735490 or d.get('role')!='observer' or not d.get('reactor_running') or d.get('peer_count',0)<1:raise ValueError('observer not ready')
   self.reply(200,{k:d[k] for k in ('network','chain_id','role','finalized_height','peer_count')})
  except Exception:self.reply(503,{'error':'observer_unavailable'})
  finally:SLOTS.release()
 def do_POST(self):
  if self.path.split('?',1)[0]!='/rpc':return self.reply(404,{'error':'not_found'})
  if self.headers.get('Transfer-Encoding') or len(self.headers.get_all('Content-Length',[]))!=1:return self.reply(400,{'error':'invalid_framing'})
  if self.headers.get_content_type()!='application/json':return self.reply(415,{'error':'application_json_required'})
  try:
   size=int(self.headers['Content-Length'])
   if not 0<size<=MAX_BODY:return self.reply(413,{'error':'request_too_large'})
   raw=self.rfile.read(size)
   if len(raw)!=size:raise ValueError('incomplete body')
   d=json.loads(raw);d=validate(d)
  except PermissionError:return self.reply(200,{'jsonrpc':'2.0','id':d.get('id'),'error':{'code':-32601,'message':'method unavailable'}})
  except (ValueError,TypeError,socket.timeout):return self.reply(400,{'jsonrpc':'2.0','id':None,'error':{'code':-32600,'message':'invalid request'}})
  if not SLOTS.acquire(False):return self.reply(503,{'error':'busy'})
  try:self.reply(200,upstream('/rpc',json.dumps(d).encode()))
  except Exception:self.reply(502,{'jsonrpc':'2.0','id':d['id'],'error':{'code':-32000,'message':'observer unavailable'}})
  finally:SLOTS.release()
if __name__=='__main__':ThreadingHTTPServer(('127.0.0.1',18010),Handler).serve_forever()
