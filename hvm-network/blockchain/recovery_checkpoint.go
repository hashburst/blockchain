package blockchain

// Recovery checkpoints are local, authenticated caches of fully validated execution.
// They are never accepted from peers and never replace signing journals. Possession
// of the node-local sealing key is a trust boundary, just like custody of its other
// private runtime files. HVM_FULL_REPLAY=1 ignores this cache for independent audit.
import (
 "bytes"
 "compress/gzip"
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha256"
 "encoding/binary"
 "encoding"
 "encoding/gob"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "io"
 "log"
 "math/big"
 "os"
 "path/filepath"
 "time"

 "github.com/ethereum/go-ethereum/common"
 "github.com/ethereum/go-ethereum/core/rawdb"
 "github.com/ethereum/go-ethereum/core/state"
 "github.com/ethereum/go-ethereum/core/types"
 "github.com/ethereum/go-ethereum/triedb"
 "github.com/ethereum/go-ethereum/trie"
 "github.com/ethereum/go-ethereum/rlp"
 "github.com/ethereum/go-ethereum/ethdb"
 "hashburst/consensus"
 execution "hashburst/evm-execution"
 "hashburst/hvm"
)

const checkpointInterval = 1024
const checkpointMaxBytes = 64 << 20
const checkpointFormat = 1 // Increment on projection/execution semantics changes.

type recoverySeed struct {
 height int
 state *State
 engine *hvm.Engine
 validators *consensus.Registry
 receipts map[string]hvm.Receipt
}
type checkpointKV struct { Key, Value []byte }
type checkpointEthHistory struct { Receipts [][]byte; Transactions [][]byte }
type checkpointEVM struct {
 Root, ReceiptsRoot string
 GasUsed uint64
 Accounts []common.Address
 Database []checkpointKV
 History []checkpointEthHistory
}
type recoveryCheckpoint struct {
 Version int
 Height int
 Hash, Config, PrefixHash, NodeBinding string
 PrefixBytes int64
 Balances map[string]int64
 Sequences map[string]uint64
 HVM map[string][]byte
 Validators []consensus.Validator
 Receipts map[string]hvm.Receipt
 EVM *checkpointEVM
}

// The seal is node-local: a copied cache is not a bootstrap mechanism.
func (bc *Blockchain) checkpointBinding() (string,error) {
 pin,err:=os.ReadFile(filepath.Join(bc.storage.dir,"runtime.pin"));if err!=nil{return "",err}
 h:=sha256.New();h.Write([]byte(bc.storage.dir));h.Write([]byte{0});h.Write(pin)
 return hex.EncodeToString(h.Sum(nil)),nil
}
type checkpointBudgetWriter struct { writer io.Writer; written int }
func (w *checkpointBudgetWriter) Write(p []byte) (int,error) {
 if len(p)>checkpointMaxBytes-w.written{return 0,fmt.Errorf("checkpoint exceeds byte budget")}
 n,err:=w.writer.Write(p);w.written+=n;return n,err
}

func (bc *Blockchain) checkpointPath(slot int) string {
 return filepath.Join(bc.storage.dir, fmt.Sprintf("recovery-checkpoint-%d.bin", slot))
}
func checkpointKey(dir string, create bool) ([]byte,error) {
 p := filepath.Join(dir, "recovery-checkpoint.key")
 st, err := os.Lstat(p)
 if os.IsNotExist(err) && create {
  key := make([]byte,32); if _,err=rand.Read(key);err!=nil{return nil,err}
  f,e:=os.OpenFile(p,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if e!=nil{return nil,e}
  _,e=f.Write(key);if e==nil{e=f.Sync()};ce:=f.Close();if e!=nil{return nil,e};if ce!=nil{return nil,ce}
  return key,nil
 }
 if err!=nil{return nil,err}
 if !st.Mode().IsRegular() || st.Mode().Perm()&0077!=0 || st.Size()!=32{return nil,fmt.Errorf("invalid local checkpoint key")}
 return os.ReadFile(p)
}
func checkpointPrefix(s *ChainStorage, height int, previousBytes int64, previousState []byte) (int64,string,[]byte,error) {
 f,err:=os.Open(s.idxPath);if err!=nil{return 0,"",nil,err}
 var rec [20]byte
 _,err=f.ReadAt(rec[:],int64(height)*20);f.Close();if err!=nil{return 0,"",nil,err}
 if binary.BigEndian.Uint64(rec[:8])!=uint64(height){return 0,"",nil,fmt.Errorf("checkpoint index height mismatch")}
 end:=binary.BigEndian.Uint64(rec[8:16])+uint64(binary.BigEndian.Uint32(rec[16:]))
 if end>uint64(^uint64(0)>>1){return 0,"",nil,fmt.Errorf("checkpoint offset overflow")}
 d,err:=os.Open(s.datPath);if err!=nil{return 0,"",nil,err};defer d.Close()
 h:=sha256.New()
 start:=int64(0)
 if previousBytes>0 && previousBytes<=int64(end) && len(previousState)>0 {
  if err=h.(encoding.BinaryUnmarshaler).UnmarshalBinary(previousState);err!=nil{return 0,"",nil,err}
  start=previousBytes
 }
 if _,err=d.Seek(start,io.SeekStart);err!=nil{return 0,"",nil,err}
 if _,err=io.CopyN(h,d,int64(end)-start);err!=nil{return 0,"",nil,err}
 encoded,err:=h.(encoding.BinaryMarshaler).MarshalBinary();if err!=nil{return 0,"",nil,err}
 return int64(end),hex.EncodeToString(h.Sum(nil)),encoded,nil
}
// Copy only trie nodes reachable from this root and referenced contract code.
// Historical trie garbage, preimages and unrelated database keys are not a checkpoint.
func compactCheckpointTrie(src state.Database, root common.Hash) (ethdb.Database,error) {
 out:=rawdb.NewMemoryDatabase()
 var used int
 copyNodes:=func(it trie.NodeIterator) error {
  for it.Next(true){
   if hash:=it.Hash(); hash!=(common.Hash{}) {
    blob:=it.NodeBlob();used+=len(blob)+32
    if used>checkpointMaxBytes{return fmt.Errorf("reachable trie exceeds checkpoint budget")}
    if err:=out.Put(hash[:],blob);err!=nil{return err}
   }
  }
  return it.Error()
 }
 accounts,err:=src.OpenTrie(root);if err!=nil{return nil,err}
 nodes,err:=accounts.NodeIterator(nil);if err!=nil{return nil,err}
 if err=copyNodes(nodes);err!=nil{return nil,err}
 nodes,err=accounts.NodeIterator(nil);if err!=nil{return nil,err}
 for nodes.Next(true){
  if !nodes.Leaf(){continue}
  var account types.StateAccount
  if err=rlp.DecodeBytes(nodes.LeafBlob(),&account);err!=nil{return nil,err}
  codeHash:=common.BytesToHash(account.CodeHash)
  if codeHash!=types.EmptyCodeHash {
   code:=rawdb.ReadCode(src.TrieDB().Disk(),codeHash)
   if len(code)==0{return nil,fmt.Errorf("checkpoint contract code missing")}
   used+=len(code)+32;if used>checkpointMaxBytes{return nil,fmt.Errorf("checkpoint code exceeds budget")}
   rawdb.WriteCode(out,codeHash,code)
  }
  if account.Root!=types.EmptyRootHash {
   storage,err:=trie.NewStateTrie(trie.StorageTrieID(root,common.BytesToHash(nodes.LeafKey()),account.Root),src.TrieDB());if err!=nil{return nil,err}
   slots,err:=storage.NodeIterator(nil);if err!=nil{return nil,err}
   if err=copyNodes(slots);err!=nil{return nil,err}
  }
 }
 if err=nodes.Error();err!=nil{return nil,err}
 return out,nil
}

func exportCheckpointEVM(p *evmProjection, b *Block, chainID uint64) (*checkpointEVM,error) {
 if p==nil{return nil,nil}
 cfg,err:=execution.Config(chainID);if err!=nil{return nil,err}
 copyDB:=p.db.Copy()
 root,err:=copyDB.Commit(cfg.Rules(big.NewInt(int64(b.Index)),true,uint64(b.Timestamp.Unix())),uint64(b.Index))
 if err!=nil{return nil,err}
 if root.Hex()!=p.root{return nil,fmt.Errorf("checkpoint EVM commit changed root")}
 tdb:=copyDB.Database().TrieDB()
 if err=tdb.Commit(root,false);err!=nil{return nil,err}
 out:=&checkpointEVM{Root:p.root,ReceiptsRoot:p.receiptsRoot,GasUsed:p.gasUsed}
 for a:=range p.accounts{out.Accounts=append(out.Accounts,a)}
 compact,err:=compactCheckpointTrie(copyDB.Database(),root);if err!=nil{return nil,err}
 it:=compact.NewIterator(nil,nil);defer it.Release()
 var size int
 for it.Next(){
  size+=len(it.Key())+len(it.Value());if size>checkpointMaxBytes{return nil,fmt.Errorf("EVM checkpoint exceeds budget")}
  out.Database=append(out.Database,checkpointKV{append([]byte(nil),it.Key()...),append([]byte(nil),it.Value()...)})
 }
 if err=it.Error();err!=nil{return nil,err}
 for h:=p.history;h!=nil;h=h.previous{
  entry:=checkpointEthHistory{}
  for _,receipt:=range h.receipts{copyReceipt:=*receipt;if copyReceipt.Logs==nil{copyReceipt.Logs=[]*types.Log{}};encoded,e:=json.Marshal(&copyReceipt);if e!=nil{return nil,e};entry.Receipts=append(entry.Receipts,encoded)}
  for _,tx:=range h.transactions{raw,e:=tx.MarshalBinary();if e!=nil{return nil,e};entry.Transactions=append(entry.Transactions,raw)}
  out.History=append(out.History,entry)
 }
 return out,nil
}
func restoreCheckpointEVM(in *checkpointEVM) (*evmProjection,error) {
 if in==nil{return nil,nil}
 disk:=rawdb.NewMemoryDatabase()
 for _,kv:=range in.Database{if err:=disk.Put(kv.Key,kv.Value);err!=nil{return nil,err}}
 db:=state.NewDatabase(triedb.NewDatabase(disk,nil),state.NewCodeDB(disk))
 st,err:=state.New(common.HexToHash(in.Root),db);if err!=nil{return nil,err}
 p:=&evmProjection{db:st,root:in.Root,receiptsRoot:in.ReceiptsRoot,gasUsed:in.GasUsed,accounts:make(map[common.Address]struct{})}
 for _,a:=range in.Accounts{p.accounts[a]=struct{}{}}
 for i:=len(in.History)-1;i>=0;i--{
  e:=in.History[i];h:=&ethereumReceipts{previous:p.history}
  for _,encoded:=range e.Receipts{receipt:=new(types.Receipt);if err=json.Unmarshal(encoded,receipt);err!=nil{return nil,err};h.receipts=append(h.receipts,receipt)}
  if len(e.Receipts)!=len(e.Transactions){return nil,fmt.Errorf("checkpoint receipt count mismatch")}
  for _,raw:=range e.Transactions{tx:=new(types.Transaction);if err=tx.UnmarshalBinary(raw);err!=nil{return nil,err};h.transactions=append(h.transactions,tx)}
  p.history=h
 }
 return p,nil
}

func (bc *Blockchain) saveRecoveryCheckpoint(height int, st *State, engine *hvm.Engine, validators *consensus.Registry, receipts map[string]hvm.Receipt) error {
 if bc.storage==nil||!bc.storage.durable||height<1||!bc.v2Config.ConsensusEnabledAt(height){return nil}
 b:=bc.Blocks[height]
 if b.FinalityCertificate==nil{return fmt.Errorf("checkpoint requires finalized block")}
 started:=time.Now()
 binding,err:=bc.checkpointBinding();if err!=nil{return err}
 evm,err:=exportCheckpointEVM(st.evm,b,bc.v2Config.ChainID);if err!=nil{return err}
 n,digest,hashState,err:=checkpointPrefix(bc.storage,height,bc.checkpointBytes,bc.checkpointHashState);if err!=nil{return err}
 cp:=recoveryCheckpoint{Version:checkpointFormat,Height:height,Hash:b.Hash,Config:bc.recoveryConfigHash(),PrefixBytes:n,PrefixHash:digest,NodeBinding:binding,Balances:st.balances,Sequences:st.sequences,HVM:engine.State().CheckpointValues(),Validators:validators.Snapshot(),Receipts:receipts,EVM:evm}
 // Encode to a bounded buffer before publishing; a failed cache write is nonfatal.
 var packed bytes.Buffer
 compressed:=&checkpointBudgetWriter{writer:&packed}
 zw,_:=gzip.NewWriterLevel(compressed,gzip.BestSpeed)
 plain:=&checkpointBudgetWriter{writer:zw}
 if err=gob.NewEncoder(plain).Encode(&cp);err!=nil{zw.Close();return err}
 if err=zw.Close();err!=nil{return err}
 key,err:=checkpointKey(bc.storage.dir,true);if err!=nil{return err}
 mac:=hmac.New(sha256.New,key);mac.Write(packed.Bytes())
 f,err:=os.CreateTemp(bc.storage.dir,".recovery-checkpoint-*");if err!=nil{return err}
 name:=f.Name();defer os.Remove(name)
 if _,err=f.Write(mac.Sum(nil));err==nil{_,err=f.Write(packed.Bytes())};if err==nil{err=f.Sync()};ce:=f.Close();if err!=nil{return err};if ce!=nil{return ce}
 if err=os.Rename(name,bc.checkpointPath((height/checkpointInterval)%2));err!=nil{return err}
 dir,err:=os.Open(bc.storage.dir);if err!=nil{return err};err=dir.Sync();dir.Close();if err!=nil{return err}
 bc.checkpointHeight=height
 bc.checkpointBytes=n;bc.checkpointHashState=hashState
 log.Printf("HVM_CHECKPOINT_SAVED height=%d raw_bytes=%d compressed_bytes=%d elapsed=%s",height,plain.written,packed.Len(),time.Since(started))
 return nil
}
func (bc *Blockchain) maybeSaveRecoveryCheckpoint() {
 if !bc.checkpointEnabled||bc.storage==nil||!bc.storage.durable{return}
 height:=len(bc.Blocks)-1
 if height%checkpointInterval!=0||height<=bc.checkpointHeight{return}
 if err:=bc.saveRecoveryCheckpoint(height,bc.state,bc.hvmEngine,bc.validators,bc.receipts);err!=nil{log.Printf("HVM_CHECKPOINT_WRITE_SKIPPED reason=%v",err)}
}
func (bc *Blockchain) readCheckpoint(slot int,key []byte) (*recoveryCheckpoint,error) {
 path:=bc.checkpointPath(slot)
 info,err:=os.Lstat(path);if err!=nil{return nil,err}
 if !info.Mode().IsRegular()||info.Size()<33||info.Size()>checkpointMaxBytes{return nil,fmt.Errorf("invalid checkpoint file")}
 data,err:=os.ReadFile(path);if err!=nil{return nil,err}
 mac:=hmac.New(sha256.New,key);mac.Write(data[32:]);if !hmac.Equal(data[:32],mac.Sum(nil)){return nil,fmt.Errorf("checkpoint authentication failed")}
 zr,err:=gzip.NewReader(bytes.NewReader(data[32:]));if err!=nil{return nil,err};defer zr.Close()
 raw,err:=io.ReadAll(io.LimitReader(zr,checkpointMaxBytes+1));if err!=nil{return nil,err};if len(raw)>checkpointMaxBytes{return nil,fmt.Errorf("checkpoint expansion exceeds budget")}
 var cp recoveryCheckpoint
 if err=gob.NewDecoder(bytes.NewReader(raw)).Decode(&cp);err!=nil{return nil,err}
 if cp.Version!=checkpointFormat||cp.Config!=bc.recoveryConfigHash()||cp.Height<1||cp.Height>=len(bc.Blocks)-evmReadHistoryLimit{return nil,fmt.Errorf("checkpoint version/config/height/window mismatch")}
 binding,err:=bc.checkpointBinding();if err!=nil{return nil,err};if cp.NodeBinding!=binding{return nil,fmt.Errorf("checkpoint belongs to another node")}
 b:=bc.Blocks[cp.Height]
 if cp.Hash!=b.Hash||b.FinalityCertificate==nil{return nil,fmt.Errorf("checkpoint head mismatch")}
 n,digest,_,err:=checkpointPrefix(bc.storage,cp.Height,0,nil);if err!=nil{return nil,err}
 if n!=cp.PrefixBytes||digest!=cp.PrefixHash{return nil,fmt.Errorf("checkpoint chain prefix changed")}
 return &cp,nil
}
func (bc *Blockchain) loadRecoveryCheckpoint() (*recoverySeed,error) {
 key,err:=checkpointKey(bc.storage.dir,false);if err!=nil{return nil,err}
 var best *recoveryCheckpoint
 for slot:=0;slot<2;slot++{cp,e:=bc.readCheckpoint(slot,key);if e==nil&&(best==nil||cp.Height>best.Height){best=cp}}
 if best==nil{return nil,fmt.Errorf("no eligible authenticated checkpoint")}
 b:=bc.Blocks[best.Height]
 st:=NewState();st.balances=best.Balances;st.sequences=best.Sequences
 if st.Root()!=b.HBTStateRoot{return nil,fmt.Errorf("checkpoint native root mismatch")}
 hs:=hvm.NewStateDB();for k,v:=range best.HVM{hs.Set(k,v)}
 if hs.Root()!=b.HVMStateRoot{return nil,fmt.Errorf("checkpoint HVM root mismatch")}
 validators,err:=consensus.RestoreCheckpoint(bc.v2Config.Validator,best.Validators,b.ValidatorStateRoot);if err!=nil{return nil,err}
 st.evm,err=restoreCheckpointEVM(best.EVM);if err!=nil{return nil,err}
 if err=checkEVMCommitments(b,st);err!=nil{return nil,err}
 return &recoverySeed{height:best.Height,state:st,engine:hvm.NewEngine(hs,bc.v2Config.FeePolicy),validators:validators,receipts:best.Receipts},nil
}

// RecoveryStatus reports startup work without exposing paths or cache secrets.
type RecoveryStatus struct {
 Mode string `json:"mode"`
 CheckpointHeight int `json:"checkpoint_height"`
 ReplayBlocks int `json:"replay_blocks"`
 ElapsedMillis int64 `json:"elapsed_ms"`
}
func (bc *Blockchain) RecoveryStatus() RecoveryStatus {
 bc.mu.RLock();defer bc.mu.RUnlock();return bc.recoveryStatus
}
