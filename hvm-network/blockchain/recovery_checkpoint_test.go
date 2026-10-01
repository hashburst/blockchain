package blockchain

import (
 "bytes"
 "encoding/hex"
 "math/big"
 "os"
 "path/filepath"
 "testing"
 "time"
 "runtime"
 "context"
 "reflect"
 "hashburst/wallet"

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
 if err:=os.WriteFile(filepath.Join(n.storage.dir,"runtime.pin"),[]byte("test-node-identity"),0600);err!=nil{t.Fatal(err)}
 finalizeEVMFixture(t,s)
 sender:=common.HexToAddress(s.vals[0].operator.Address())
 key,err:=crypto.ToECDSA(s.vals[0].operator.PrivateKeyBytes());if err!=nil{t.Fatal(err)}
 nonce:=n.state.Sequence(sender.Hex())
 // Contract stores 42 in slot zero, returns it on call; nonempty code/storage must survive.
 code,_:=hex.DecodeString("60005460005260006000a060206000f3")
 prefix,_:=hex.DecodeString("602a600055")
 init:=append(prefix,[]byte{0x60,byte(len(code)),0x60,17,0x60,0,0x39,0x60,byte(len(code)),0x60,0,0xf3}...)
 init=append(init,code...)
 tx:=types.NewTx(&types.DynamicFeeTx{ChainID:new(big.Int).SetUint64(cfg.ChainID),Nonce:nonce,Gas:200000,GasFeeCap:big.NewInt(3),GasTipCap:big.NewInt(1),Data:init})
 tx,err=types.SignTx(tx,types.NewCancunSigner(new(big.Int).SetUint64(cfg.ChainID)),key);if err!=nil{t.Fatal(err)}
 raw,_:=tx.MarshalBinary()
 for _,node:=range s.nodes{if _,err=node.AdmitEthereum(raw);err!=nil{t.Fatal(err)}}
 finalizeEVMFixture(t,s)
 contract:=crypto.CreateAddress(sender,nonce)
 if n.state.evm.db.GetState(contract,common.Hash{})!=common.BigToHash(big.NewInt(42)){t.Fatal("fixture storage missing")}
 call:=types.NewTx(&types.DynamicFeeTx{ChainID:new(big.Int).SetUint64(cfg.ChainID),Nonce:nonce+1,To:&contract,Gas:100000,GasFeeCap:big.NewInt(3),GasTipCap:big.NewInt(1)})
 call,err=types.SignTx(call,types.NewCancunSigner(new(big.Int).SetUint64(cfg.ChainID)),key);if err!=nil{t.Fatal(err)}
 callRaw,_:=call.MarshalBinary();for _,node:=range s.nodes{if _,err=node.AdmitEthereum(callRaw);err!=nil{t.Fatal(err)}}
 finalizeEVMFixture(t,s)
 if err=n.saveRecoveryCheckpoint(n.Height(),n.state,n.hvmEngine,n.validators,n.receipts);err!=nil{t.Fatal(err)}
 for i:=0;i<768;i++{finalizeEVMFixture(t,s)}
 if err=n.saveRecoveryCheckpoint(n.Height(),n.state,n.hvmEngine,n.validators,n.receipts);err!=nil{t.Fatal(err)}
 cpHeight:=n.Height()
 for i:=0;i<300;i++{finalizeEVMFixture(t,s)}
 journal:=filepath.Join(n.storage.dir,"consensus-bft-signatures.jsonl")
 before,_:=os.ReadFile(journal)
 open:=func()*Blockchain{t.Helper();b,e:=OpenExistingBlockchainWithRecovery(n.storage.dir,n.v2Config,n.Blocks[0].Hash,6,n.Blocks[6].Hash);if e!=nil{t.Fatal(e)};return b}
 var m0,m1 runtime.MemStats
 runtime.GC();runtime.ReadMemStats(&m0)
 start:=time.Now();fast:=open();fastTime:=time.Since(start)
 runtime.ReadMemStats(&m1);fastAlloc:=m1.TotalAlloc-m0.TotalAlloc
 if fast.RecoveryStatus().Mode!="incremental"||fast.RecoveryStatus().CheckpointHeight!=cpHeight{t.Fatal("checkpoint was not restored")}
 if fast.state.Root()!=n.state.Root()||fast.state.evm.root!=n.state.evm.root||fast.validators.Root()!=n.validators.Root()||fast.HVMStateRoot()!=n.HVMStateRoot(){t.Fatal("incremental roots differ")}
 if !bytes.Equal(fast.state.evm.db.GetCode(contract),n.state.evm.db.GetCode(contract))||fast.state.evm.db.GetState(contract,common.Hash{})!=common.BigToHash(big.NewInt(42)){t.Fatal("contract checkpoint mismatch")}
 receipt,_:=fast.ethereumReceiptLocked(tx.Hash());original,_:=n.ethereumReceiptLocked(tx.Hash())
 if receipt==nil||receipt.Status!=1||receipt.TxHash!=original.TxHash||receipt.GasUsed!=original.GasUsed||receipt.ContractAddress!=original.ContractAddress||receipt.BlockHash!=original.BlockHash{t.Fatal("receipt lost across checkpoint")}
 callReceipt,_:=fast.ethereumReceiptLocked(call.Hash());originalCall,_:=n.ethereumReceiptLocked(call.Hash())
 if callReceipt==nil||len(callReceipt.Logs)!=1||!reflect.DeepEqual(callReceipt.Logs,originalCall.Logs){t.Fatal("checkpoint log metadata mismatch")}
 if _,e:=fast.evmReadHistory.snapshot(fast.Blocks[len(fast.Blocks)-256]);e!=nil{t.Fatal(e)}
 t.Setenv("HVM_FULL_REPLAY","1");runtime.GC();runtime.ReadMemStats(&m0);start=time.Now();full:=open();fullTime:=time.Since(start)
 runtime.ReadMemStats(&m1);fullAlloc:=m1.TotalAlloc-m0.TotalAlloc
 if full.RecoveryStatus().Mode!="full" { t.Fatal("audit did not force full verification") }
 if full.state.Root()!=fast.state.Root()||full.state.evm.root!=fast.state.evm.root{t.Fatal("full replay mismatch")}
 after,_:=os.ReadFile(journal);if !bytes.Equal(before,after){t.Fatal("checkpoint modified signing journal")}
 t.Logf("CHECKPOINT_TIMING blocks=%d fast=%s full=%s fast_allocated_bytes=%d full_allocated_bytes=%d (fixture, not peak RSS, VPS or energy measurement)",len(n.Blocks),fastTime,fullTime,fastAlloc,fullAlloc)
 t.Setenv("HVM_FULL_REPLAY","")
 // A valid seal is still rejected for another node or changed canonical prefix.
 pinPath:=filepath.Join(n.storage.dir,"runtime.pin")
 pin,_:=os.ReadFile(pinPath);os.WriteFile(pinPath,[]byte("different-node"),0600)
 if _,e:=n.loadRecoveryCheckpoint();e==nil{t.Fatal("another node identity accepted")}
 os.WriteFile(pinPath,pin,0600)
 chainBytes,_:=os.ReadFile(n.storage.datPath)
 modified:=append([]byte(nil),chainBytes...);modified[20]^=1
 os.WriteFile(n.storage.datPath,modified,0600)
 if _,e:=n.loadRecoveryCheckpoint();e==nil{t.Fatal("changed chain prefix accepted")}
 os.WriteFile(n.storage.datPath,chainBytes,0600)
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

func TestRecoveryCheckpointAPoWBothNetworks(t *testing.T) {
 for _,chainID:=range []uint64{4735490,4735489}{
  cfg:=apowTestConfig();cfg.ChainID=chainID
  s:=setupPhase3DChainsWithConfig(t,cfg);n:=s.nodes[0];n.storage.durable=true
  os.WriteFile(filepath.Join(n.storage.dir,"runtime.pin"),[]byte("apow-fixture"),0600)
  miner,err:=wallet.NewWallet();if err!=nil{t.Fatal(err)}
  for i:=0;i<270;i++{
   job,e:=n.APoWJob();if e!=nil{t.Fatal(e)}
   ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
   proof,e:=MineAPoW(ctx,*job,miner,miner.Address());cancel();if e!=nil{t.Fatal(e)}
   for _,node:=range s.nodes{if e=node.SubmitAPoW(*proof);e!=nil{t.Fatal(e)}}
   b:=finalizeEVMFixture(t,s)
   if e=validateRewardRecipient(b,miner.Address());e!=nil||b.Transactions[0].Amount!=50{t.Fatal("reward mismatch",e)}
   if i==0{if e=n.saveRecoveryCheckpoint(b.Index,n.state,n.hvmEngine,n.validators,n.receipts);e!=nil{t.Fatal(e)}}
  }
  recovered,e:=OpenExistingBlockchainWithRecovery(n.storage.dir,n.v2Config,n.Blocks[0].Hash,6,n.Blocks[6].Hash);if e!=nil{t.Fatal(e)}
  if recovered.RecoveryStatus().Mode!="incremental"||recovered.state.Root()!=n.state.Root()||recovered.state.evm.root!=n.state.evm.root{t.Fatal("APoW incremental recovery mismatch")}
  if recovered.state.BalanceUnits(miner.Address())!=270*50*AmountScale{t.Fatal("APoW emission mismatch")}
 }
}
