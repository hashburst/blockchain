import importlib.util
import io
import json
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import patch, MagicMock

spec=importlib.util.spec_from_file_location('preflight',Path(__file__).parents[1]/'preflight-node.py')
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)

class PreflightTests(unittest.TestCase):
    def run_check(self,ready=True,key='ab'*32,busy=False,existing=False):
        calls=[]
        def command(args,**kwargs):
            calls.append(args)
            if args[0]=='ip':
                return SimpleNamespace(returncode=0,stdout=json.dumps([{'addr_info':[{'family':'inet','local':'77.90.188.153'}]}]))
            return SimpleNamespace(returncode=0,stdout='active\n' if args[1]=='is-active' else 'inactive\n')
        payload={'node_id':'hashburst-dr1','pubkey':key,'app_ready':ready,'peers':[{'online':True}]}
        response=MagicMock();response.__enter__.return_value.read.return_value=json.dumps(payload).encode()
        opener=MagicMock();opener.open.return_value=response
        sock=MagicMock();sock.__enter__.return_value.bind.side_effect=OSError('busy') if busy else None
        with patch.object(module.subprocess,'run',side_effect=command),patch.object(module.platform,'system',return_value='Linux'),patch.object(module.platform,'machine',return_value='x86_64'),patch.object(module.os,'geteuid',return_value=0),patch.object(module.urllib.request,'build_opener',return_value=opener),patch.object(module.socket,'socket',return_value=sock),patch.object(Path,'exists',return_value=existing),patch.object(Path,'is_symlink',return_value=False):
            result=module.check('77.90.188.153')
        self.assertTrue(all(c[0]=='ip' or c[:2] in (['systemctl','is-active'],['systemctl','show']) for c in calls))
        return result
    def test_valid_public_identity(self):
        r=self.run_check();self.assertTrue(r['ok']);self.assertEqual(r['tep_public_key'],'ab'*32)
    def test_missing_authenticated_tep_fails(self):
        self.assertFalse(self.run_check(ready=False)['ok'])
    def test_invalid_public_key_fails(self):
        r=self.run_check(key='00'*32);self.assertFalse(r['ok']);self.assertNotIn('tep_public_key',r)
    def test_occupied_ports_fail(self):
        self.assertFalse(self.run_check(busy=True)['ok'])
    def test_existing_state_fails_without_removal(self):
        r=self.run_check(existing=True);self.assertFalse(r['ok']);self.assertEqual(len(r['existing_paths']),3)

if __name__=='__main__':unittest.main()
