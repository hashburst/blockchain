import copy, importlib.util, unittest
from pathlib import Path
spec=importlib.util.spec_from_file_location('verify',Path(__file__).with_name('verify-public.py'));v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
class Tests(unittest.TestCase):
 def setUp(self):
  self.log=dict(address='0xcontract',data='0x',topics=[],logIndex='0x0',transactionHash='call',blockHash='block')
  self.receipts={label:dict(transactionHash=label,blockHash='block',blockNumber='0x10',status='0x1',gasUsed='0x5208',contractAddress='0xcontract' if label=='deploy' else None,logs=[self.log] if label=='call' else []) for label in ['transfer','deploy','call']}
  self.proof=dict(result='METAMASK_TESTNET_MANUAL_CANARY_OK',chainId=4735490,endpoint=v.RPC,account='0xaccount',checks=[dict(label=k,hash=k,receipt=copy.deepcopy(r)) for k,r in self.receipts.items()],websocket=dict(unsubscribed=True,head=dict(result=dict(hash='block')),log=dict(result=copy.deepcopy(self.log))))
  def rpc(method,params=None):
   if method=='eth_getBlockByNumber':return dict(hash='block',number='0x20')
   if method=='eth_getTransactionReceipt':return self.receipts[params[0]]
   if method=='eth_getTransactionByHash':return dict(chainId=hex(4735490),**{'from':'0xaccount'})
   if method=='eth_getCode':return '0x602a60005560006000a000'
   if method=='eth_getStorageAt':return '0x2a'
   if method=='eth_getLogs':return [self.log]
   raise AssertionError(method)
  v.rpc=rpc
 def test_valid_proof(self):self.assertTrue(v.check_wallet(self.proof)['wallet_websocket_evidence_verified'])
 def test_unfinalized_receipt(self):
  for r in self.receipts.values():r['blockNumber']='0x30'
  for c in self.proof['checks']:c['receipt']['blockNumber']='0x30'
  with self.assertRaisesRegex(RuntimeError,'not finalized'):v.check_wallet(self.proof)
 def test_wrong_chain(self):
  self.proof['chainId']=4735489
  with self.assertRaisesRegex(RuntimeError,'network'):v.check_wallet(self.proof)
 def test_fake_subscription(self):
  self.proof['websocket']['log']['result']['data']='0x01'
  with self.assertRaisesRegex(RuntimeError,'canonical log'):v.check_wallet(self.proof)
 def test_altered_receipt(self):
  self.proof['checks'][0]['receipt']['gasUsed']='0x1'
  with self.assertRaisesRegex(RuntimeError,'receipt differs'):v.check_wallet(self.proof)
if __name__=='__main__':unittest.main()
