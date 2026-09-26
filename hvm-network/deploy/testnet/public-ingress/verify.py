#!/usr/bin/env python3
import base64,hashlib,json,os,socket,ssl,struct,urllib.request
HOST='blockchainapi.one';PREFIX='/api/hashburst/hvm/testnet'
def rpc(method):
 q=urllib.request.Request('https://'+HOST+PREFIX+'/rpc',json.dumps(dict(jsonrpc='2.0',id=1,method=method,params=[])).encode(),{'Content-Type':'application/json'})
 with urllib.request.urlopen(q,timeout=25) as r:return json.load(r)
def exact(s,n):
 b=b''
 while len(b)<n:
  c=s.recv(n-len(b))
  if not c:raise RuntimeError('WebSocket closed early')
  b+=c
 return b
def ws():
 with socket.create_connection((HOST,443),timeout=15) as raw:
  with ssl.create_default_context().wrap_socket(raw,server_hostname=HOST) as s:
   key=base64.b64encode(os.urandom(16)).decode()
   s.sendall(('GET '+PREFIX+'/ws HTTP/1.1\r\nHost: '+HOST+'\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: '+key+'\r\nOrigin: https://'+HOST+'\r\n\r\n').encode())
   headers=b''
   while not headers.endswith(b'\r\n\r\n'):
    headers+=exact(s,1)
    if len(headers)>16384:raise RuntimeError('oversized handshake')
   if b' 101 ' not in headers.split(b'\r\n')[0]:raise RuntimeError(headers.decode())
   expected=base64.b64encode(hashlib.sha1((key+'258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
   if ('sec-websocket-accept: '+expected).lower() not in headers.decode().lower():raise RuntimeError('invalid WebSocket handshake')
   for method in ['eth_chainId','hb_sendRawTransactionV2','eth_subscribe']:
    data=json.dumps(dict(jsonrpc='2.0',id=1,method=method,params=[])).encode();mask=os.urandom(4)
    if len(data)>=126:raise RuntimeError('test frame too long')
    s.sendall(bytes([0x81,0x80|len(data)])+mask+bytes(v^mask[i%4] for i,v in enumerate(data)))
    h=exact(s,2);n=h[1]&127
    if h[0]!=0x81 or h[1]&128:raise RuntimeError('unexpected server frame')
    if n==126:n=struct.unpack('!H',exact(s,2))[0]
    elif n==127:n=struct.unpack('!Q',exact(s,8))[0]
    if n>65536:raise RuntimeError('oversized response')
    d=json.loads(exact(s,n))
    if method=='eth_chainId':assert d.get('result')=='0x484202',d
    else:assert d.get('error',{}).get('code')==-32601,d
if __name__=='__main__':
 with urllib.request.urlopen('https://'+HOST+PREFIX+'/health',timeout=25) as r:h=json.load(r)
 assert h['chain_id']==4735490 and h['role']=='observer',h
 assert rpc('eth_chainId')['result']=='0x484202'
 for method in ['hb_sendTransaction','eth_sendRawTransaction','eth_subscribe']:
  d=rpc(method);assert d.get('error',{}).get('code')==-32601,d
 print('PUBLIC_HTTPS_FILTER_OK')
 ws();print('PUBLIC_WEBSOCKET_READ_ONLY_OK\nEVM_SUBSCRIPTIONS_NOT_IMPLEMENTED\nHVM_TESTNET_PUBLIC_READ_ONLY_INGRESS_OK')
