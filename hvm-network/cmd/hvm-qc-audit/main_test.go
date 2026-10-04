package main

import (
 "testing"
 "strings"
 "hashburst/consensus"
 "hashburst/wallet"
)

func fixture(t *testing.T) Evidence {
 t.Helper()
 set:=consensus.ValidatorSet{Height:10}
 signers:=[]*wallet.Wallet{}
 for i:=0;i<4;i++ {
  w,err:=wallet.NewWallet();if err!=nil{t.Fatal(err)}
  id,err:=consensus.ValidatorID(w.PublicKeyHexCompressed());if err!=nil{t.Fatal(err)}
  set.Validators=append(set.Validators,consensus.Validator{ID:id,ConsensusPubKey:w.PublicKeyHexCompressed(),ConsensusAddress:w.Address()})
  set.Power=append(set.Power,1);signers=append(signers,w)
 }
 hash:=strings.Repeat("ab",32)
 votes:=[]consensus.Vote{}
 for i:=0;i<3;i++ {v,err:=consensus.NewSignedVote(4735490,10,0,hash,set.Root(),set.Validators[i].ID,signers[i]);if err!=nil{t.Fatal(err)};votes=append(votes,v)}
 q,err:=consensus.BuildQuorumCertificate(4735490,10,0,hash,set.Root(),set,votes);if err!=nil{t.Fatal(err)}
 return Evidence{4735490,10,hash,set.Root(),q,set}
}

func TestOfflineCertificate(t *testing.T){
 e:=fixture(t);if err:=verify(e);err!=nil{t.Fatal(err)}
 e.Certificate.Votes[0],e.Certificate.Votes[2]=e.Certificate.Votes[2],e.Certificate.Votes[0]
 if err:=verify(e);err!=nil{t.Fatal(err)}
}
func TestRejectMutation(t *testing.T){
 for _,name:=range []string{"signature","duplicate","power","address","root","height","quorum"}{t.Run(name,func(t *testing.T){
  e:=fixture(t)
  switch name {
  case "signature":e.Certificate.Votes[0].Signature=strings.Repeat("00",65)
  case "duplicate":e.Certificate.Votes[1]=e.Certificate.Votes[0]
  case "power":e.Certificate.TotalPower++
  case "address":e.Set.Validators[0].ConsensusAddress=e.Set.Validators[1].ConsensusAddress
  case "root":e.ValidatorSetRoot=strings.Repeat("00",32)
  case "height":e.Height++
  case "quorum":e.Certificate.Votes=e.Certificate.Votes[:2]
  }
  if verify(e)==nil{t.Fatal("accepted invalid evidence")}
 })}
}
