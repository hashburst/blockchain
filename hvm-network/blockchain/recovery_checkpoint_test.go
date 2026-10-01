package blockchain

import (
 "bytes"
 "encoding/hex"
 "math/big"
 "os"
 "path/filepath"
 "testing"
 "time"

 "github.com/ethereum/go-ethereum/common"
 "github.com/ethereum/go-ethereum/core/types"
 "github.com/ethereum/go-ethereum/crypto"
 execution "hashburst/evm-execution"
)

func TestRecoveryCheckpointIncrementalEVMAndTamper(t *testing.T) {
 cfg:=phase3CTestConfig();cfg.ChainID=execution.TestnetID
 cfg.EVM=&EVMConfig{ActivationHeight:7,GasLimit:1000000,BaseFeeWei:1}
 s:=setupPhase3DChainsWithConfig(t,cfg);n:=s.nodes[0]
 for _,node:=range s.nodes{node.SetMempool(NewMempool())}
 n.storage.durable=true
 finalizeEVMFixture(t,s)
 sender:=common.HexToAddress(s.vals[0].operator.Address())
 key,err:=crypto.ToECDSA(s.vals[0].operator.PrivateKeyBytes());if err!=nil{t.Fatal(err)}
 nonce:=n.state.Sequence(sender.Hex())
 // Contract stores 42 in slot zero, returns it on call; nonempty code/storage must survive.
 init,_:=hex.DecodeString("602a600055600b6011600039600b6000f360005460005260206000f3")
 tx:=types.NewTx(&types.DynamicFeeTx{ChainID:new(big.Int).SetUint64(cfg.ChainID),Nonce:nonce,Gas:200000,GasFeeCap:big.NewInt(3),GasTipCap:big.NewInt(1),Data:init})
 tx,err=types.SignTx(tx,types.NewCancunSigner(new(big.Int).SetUint64(cfg.ChainID)),key);if err!=nil{t.Fatal(err)}
 raw,_:=tx.MarshalBinary()
 for _,node:=range s.nodes{if _,err=node.AdmitEthereum(raw);err!=nil{t.Fatal(err)}}
 finalizeEVMFixture(t,s)
 contract:=crypto.CreateAddress(sender,nonce)
 if n.state.evm.db.GetState(contract,common.Hash{})!=common.BigToHash(big.NewInt(42)){t.Fatal("fixture storage missing")}
 if err=n.saveRecoveryCheckpoint(n.Height(),n.state,n.hvmEngine,n.validators,n.receipts);err!=nil{t.Fatal(err)}
 cpHeight:=n.Height()
 for i:=0;i<300;i++{finalizeEVMFixture(t,s)}
 journal:=filepath.Join(n.storage.dir,"consensus-bft-signatures.jsonl")
 before,_:=os.ReadFile(journal)
 open:=func()*Blockchain{t.Helper();b,e:=OpenExistingBlockchain(n.storage.dir,n.v2Config,n.Blocks[0].Hash,6,n.Blocks[6].Hash);if e!=nil{t.Fatal(e)};return b}
 start:=time.Now();fast:=open();fastTime:=time.Since(start)
 if fast.checkpointHeight<cpHeight{t.Fatal("checkpoint was not restored")}
 if fast.state.Root()!=n.state.Root()||fast.state.evm.root!=n.state.evm.root||fast.validators.Root()!=n.validators.Root()||fast.HVMStateRoot()!=n.HVMStateRoot(){t.Fatal("incremental roots differ")}
 if !bytes.Equal(fast.state.evm.db.GetCode(contract),n.state.evm.db.GetCode(contract))||fast.state.evm.db.GetState(contract,common.Hash{})!=common.BigToHash(big.NewInt(42)){t.Fatal("contract checkpoint mismatch")}
 receipt,_:=fast.ethereumReceiptLocked(tx.Hash());original,_:=n.ethereumReceiptLocked(tx.Hash())
 if receipt==nil||receipt.Status!=1||receipt.TxHash!=original.TxHash||receipt.GasUsed!=original.GasUsed{t.Fatal("receipt lost across checkpoint")}
 if _,e:=fast.evmReadHistory.snapshot(fast.Blocks[len(fast.Blocks)-256]);e!=nil{t.Fatal(e)}
 t.Setenv("HVM_FULL_REPLAY","1");start=time.Now();full:=open();fullTime:=time.Since(start)
 if full.state.Root()!=fast.state.Root()||full.state.evm.root!=fast.state.evm.root{t.Fatal("full replay mismatch")}
 after,_:=os.ReadFile(journal);if !bytes.Equal(before,after){t.Fatal("checkpoint modified signing journal")}
 t.Logf("CHECKPOINT_TIMING blocks=%d fast=%s full=%s (fixture, not VPS or energy measurement)",len(n.Blocks),fastTime,fullTime)
 t.Setenv("HVM_FULL_REPLAY","")
 // Corrupt both slots: authentication failure must trigger verified full replay.
 for slot:=0;slot<2;slot++{if b,e:=os.ReadFile(n.checkpointPath(slot));e==nil{b[len(b)/2]^=1;os.WriteFile(n.checkpointPath(slot),b,0600)}}
 if fallback:=open();fallback.state.Root()!=full.state.Root(){t.Fatal("corrupt checkpoint fallback mismatch")}
 // Lost key is also a cache miss, never a genesis reset or journal reset.
 if err=os.Remove(filepath.Join(n.storage.dir,"recovery-checkpoint.key"));err!=nil{t.Fatal(err)}
 if fallback:=open();fallback.state.Root()!=full.state.Root(){t.Fatal("lost key fallback mismatch")}
}

func TestRecoveryCheckpointKeyPermissionsAndIndexBounds(t *testing.T) {
 dir:=t.TempDir()
 if _,err:=checkpointKey(dir,false);err==nil{t.Fatal("missing key accepted")}
 key,err:=checkpointKey(dir,true);if err!=nil||len(key)!=32{t.Fatal(err)}
 path:=filepath.Join(dir,"recovery-checkpoint.key")
 os.Chmod(path,0644)
 if _,err=checkpointKey(dir,false);err==nil{t.Fatal("public key mode accepted")}
 os.Remove(path);os.Symlink(filepath.Join(dir,"elsewhere"),path)
 if _,err=checkpointKey(dir,true);err==nil{t.Fatal("symlink accepted")}
}
