import importlib.util
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

def load(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

installer = load('install-observer')

class Guards(unittest.TestCase):
    def config(self):
        return dict(node_id='hvm-testnet-ingress', role='observer', network='testnet', protocol=dict(chain_id=4735490, evm={}), data_dir=str(installer.DATA), rpc_listen='127.0.0.1:18009')
    def test_observer_only(self):
        c = self.config()
        installer.validate_config(c)
        for field, value in [('role', 'validator'), ('node_id', 'hvm-testnet-v1'), ('consensus_key_file', '/private/key'), ('data_dir', '/var/lib/hashburst')]:
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                installer.validate_config(dict(c, **{field: value}))
    def test_mainnet_and_legacy_rejected(self):
        for chain in (4735489, 1337):
            c = self.config(); c['protocol']['chain_id'] = chain
            with self.assertRaises(RuntimeError): installer.validate_config(c)
    def test_configuration_pin_and_journal_immutable(self):
        with tempfile.TemporaryDirectory() as folder:
            root=Path(folder); cfg=root/'node.json'; cfg.write_text('{}')
            with patch.object(installer,'DATA',root), patch.object(installer,'CONFIG',cfg):
                for name in installer.NAMES: (root/name).write_bytes(b'original')
                proof=dict(config_sha256=installer.sha(cfg), files=installer.file_proofs())
                installer.preserved(proof)
                for name in installer.NAMES:
                    (root/name).write_bytes(b'changed')
                    with self.assertRaises(RuntimeError): installer.preserved(proof)
                    (root/name).write_bytes(b'original')
                cfg.write_text('{"changed":true}')
                with self.assertRaises(RuntimeError): installer.preserved(proof)
    def test_progress_requires_identity_and_advance(self):
        p=dict(peer_id='peer',config_digest='digest',finalized_before=20)
        h=dict(chain_id=4735490,role='observer',node_id='hvm-testnet-ingress',peer_id='peer',config_digest='digest',ok=True,peer_count=1,finalized_height=20)
        self.assertFalse(installer.check_health(h,p))
        h['finalized_height']=21
        self.assertTrue(installer.check_health(h,p))
        h['role']='validator'
        with self.assertRaises(RuntimeError): installer.check_health(h,p)

if __name__ == '__main__': unittest.main()
