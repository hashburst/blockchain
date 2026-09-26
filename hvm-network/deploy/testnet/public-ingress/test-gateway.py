import unittest,json,threading,urllib.request,urllib.error
from unittest.mock import patch
import gateway as g
class Tests(unittest.TestCase):
 def test_filter(self):
  for method in ('hb_sendTransaction','hb_sendRawTransactionV2','eth_sendRawTransaction','eth_subscribe','admin_peers'):
   with self.assertRaises(PermissionError):g.validate(dict(jsonrpc='2.0',id=1,method=method))
 def test_invalid(self):
  for d in ([],{},dict(jsonrpc='2.0',method='eth_chainId'),dict(jsonrpc='2.0',id=True,method='eth_chainId')):
   with self.assertRaises(ValueError):g.validate(d)
 def test_http_denial_never_forwards(self):
  server=g.ThreadingHTTPServer(('127.0.0.1',0),g.Handler)
  thread=threading.Thread(target=server.serve_forever);thread.start()
  try:
   with patch.object(g,'upstream') as forward:
    request=urllib.request.Request('http://127.0.0.1:'+str(server.server_port)+'/rpc',json.dumps(dict(jsonrpc='2.0',id=1,method='hb_sendTransaction')).encode(),{'Content-Type':'application/json'})
    with urllib.request.urlopen(request) as r:self.assertEqual(json.load(r)['error']['code'],-32601)
    forward.assert_not_called()
  finally:server.shutdown();server.server_close();thread.join()
if __name__=='__main__':unittest.main()
