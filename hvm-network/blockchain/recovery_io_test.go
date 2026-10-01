package blockchain

import (
 "bytes"
 "encoding/binary"
 "os"
 "path/filepath"
 "testing"
)

func TestRecoveryIndexedLoaderRejectsCorruption(t *testing.T) {
 n:=NewBlockchainWithDirAndV2Config(t.TempDir(),phase3CTestConfig())
 n.storage.durable=true
 index,err:=os.ReadFile(n.storage.idxPath);if err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile(n.storage.datPath);if err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{name string; data,index []byte}{
  {"short frame",original[:len(original)-1],index},
  {"short index",original,index[:len(index)-1]},
  {"extra index",original,append(append([]byte(nil),index...),0)},
  {"wrong offset",original,func()[]byte{b:=append([]byte(nil),index...);b[15]^=1;return b}()},
  {"oversized frame",func()[]byte{b:=append([]byte(nil),original...);binary.BigEndian.PutUint32(b,^uint32(0));return b}(),index},
 }{
  t.Run(tc.name,func(t *testing.T){os.WriteFile(n.storage.datPath,tc.data,0600);os.WriteFile(n.storage.idxPath,tc.index,0600);if _,e:=n.storage.LoadAll();e==nil{t.Fatal("corrupt storage accepted")}})
 }
 os.WriteFile(n.storage.datPath,original,0600);os.WriteFile(n.storage.idxPath,index,0600)
 if _,err=n.storage.LoadAll();err!=nil{t.Fatal(err)}
 size,digest,hashState,err:=checkpointPrefix(n.storage,0,0,nil);if err!=nil{t.Fatal(err)}
 size2,digest2,_,err:=checkpointPrefix(n.storage,0,size,hashState)
 if err!=nil||size!=size2||digest!=digest2{t.Fatal("incremental prefix hash differs",err)}
}

func TestRecoveryCheckpointCodecBudget(t *testing.T){
 var out bytes.Buffer
 w:=checkpointBudgetWriter{writer:&out,written:checkpointMaxBytes-3}
 if _,e:=w.Write([]byte{1,2,3,4});e==nil{t.Fatal("budget exceeded without failure")}
 if out.Len()!=0{t.Fatal("oversized write was partially published")}
 dir:=t.TempDir();target:=filepath.Join(dir,"key")
 os.WriteFile(target,make([]byte,32),0600)
 os.Symlink(target,filepath.Join(dir,"recovery-checkpoint.key"))
 if _,e:=checkpointKey(dir,false);e==nil{t.Fatal("key symlink accepted")}
}
