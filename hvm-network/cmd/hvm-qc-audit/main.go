// hvm-qc-audit verifies certificates offline against externally anchored block
// hashes and validator-set roots. It never connects to or mutates a node.
package main

import (
 "encoding/json"
 "fmt"
 "io"
 "os"
 "strings"
 "hashburst/consensus"
 "hashburst/wallet"
)

type Evidence struct {
 ChainID uint64 `json:"chain_id"`
 Height uint64 `json:"height"`
 Hash string `json:"hash"`
 ValidatorSetRoot string `json:"validator_set_root"`
 Certificate consensus.QuorumCertificate `json:"certificate"`
 Set consensus.ValidatorSet `json:"set"`
}

func verify(e Evidence) error {
 q := e.Certificate
 if e.ChainID == 0 || e.Hash == "" || e.ValidatorSetRoot == "" || q.ChainID != e.ChainID || q.Height != e.Height || !strings.EqualFold(q.BlockHash,e.Hash) || !strings.EqualFold(q.ValidatorSetRoot,e.ValidatorSetRoot) {
  return fmt.Errorf("certificate does not match external commitment")
 }
 if len(e.Set.Validators)==0 || len(e.Set.Validators)!=len(e.Set.Power) { return fmt.Errorf("invalid set dimensions") }
 seen := map[string]bool{}
 var total uint64
 for i,v := range e.Set.Validators {
  id,err := consensus.ValidatorID(v.ConsensusPubKey)
  if err != nil { return err }
  _,address,err := wallet.CanonicalPublicKeyHex(v.ConsensusPubKey)
  if err != nil { return err }
  if !strings.EqualFold(id,v.ID) || !wallet.AddressEqual(address,v.ConsensusAddress) { return fmt.Errorf("validator identity/key binding mismatch") }
  id=strings.ToLower(id)
  if seen[id] || e.Set.Power[i]==0 || ^uint64(0)-total < e.Set.Power[i] { return fmt.Errorf("duplicate validator, zero power or overflow") }
  seen[id]=true; total+=e.Set.Power[i]
 }
 claimedSigned,claimedTotal := q.SignedPower,q.TotalPower
 if err:=q.Verify(e.Set); err!=nil { return err }
 if q.SignedPower!=claimedSigned || q.TotalPower!=claimedTotal { return fmt.Errorf("claimed quorum power differs") }
 return nil
}

func audit(input io.Reader, output io.Writer) error {
 raw,err:=io.ReadAll(io.LimitReader(input,16*1024*1024+1)); if err!=nil{return err}
 if len(raw)>16*1024*1024{return fmt.Errorf("input exceeds 16 MiB")}
 var evidence []Evidence
 if err=json.Unmarshal(raw,&evidence);err!=nil{return err}
 if len(evidence)==0{return fmt.Errorf("empty evidence")}
 for i,e:=range evidence {if err=verify(e);err!=nil{return fmt.Errorf("certificate %d: %w",i,err)}}
 return json.NewEncoder(output).Encode(map[string]interface{}{"certificates_verified":len(evidence),"signatures_and_quorum_verified":true,"scope":"relative to supplied external commitments; does not authenticate genesis, legacy balances or supply"})
}

func main(){if err:=audit(os.Stdin,os.Stdout);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
