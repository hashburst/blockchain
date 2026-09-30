import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('prompt',Path(__file__).with_name('prompt.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)

class PromptTests(unittest.TestCase):
 def test_create_never_passes_password_in_arguments(self):
  def execute(command,**kw):
   self.assertNotIn('--in',command)
   self.assertNotIn('long-test-password',command)
   self.assertEqual(kw['input'],'long-test-password\n')
   return type('Result',(),{'returncode':0})()
  with patch.object(module.sys,'argv',['prompt.py','create','--binary','wallet','--chain-id','4735490','--out','key.json']),patch.object(module.sys.stdin,'isatty',return_value=True),patch.object(module.getpass,'getpass',return_value='long-test-password'),patch.object(module.subprocess,'run',side_effect=execute):
   with self.assertRaises(SystemExit) as e:module.main()
   self.assertEqual(e.exception.code,0)
 def test_signed_snapshot_is_the_reviewed_document(self):
  with tempfile.TemporaryDirectory() as td:
   draft=Path(td)/'draft.json';original=b'{"chain_id":4735490,"value_units":100}'
   draft.write_bytes(original)
   def confirm(_):
    draft.write_text('{"chain_id":4735490,"value_units":999}')
    return 'SIGN 4735490'
   def execute(command,**kw):
    reviewed=Path(command[command.index('--draft')+1])
    self.assertEqual(reviewed.read_bytes(),original)
    self.assertEqual(os.stat(reviewed).st_mode&0o777,0o600)
    return type('Result',(),{'returncode':0})()
   with patch.object(module.sys,'argv',['prompt.py','sign-transfer','--binary','wallet','--chain-id','4735490','--source','key','--address','public','--out','signed','--draft',str(draft)]),patch.object(module.sys.stdin,'isatty',return_value=True),patch('builtins.input',side_effect=confirm),patch.object(module.getpass,'getpass',return_value='long-test-password'),patch.object(module.subprocess,'run',side_effect=execute):
    with self.assertRaises(SystemExit) as e:module.main()
    self.assertEqual(e.exception.code,0)

if __name__=='__main__':unittest.main()
