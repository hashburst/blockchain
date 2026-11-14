package faucet

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hashburst/protocolv2"
	"hashburst/wallet"
)

type fakeNode struct{ sender string; seq uint64; submitted int; fail bool }
func (n *fakeNode) Prepare(to string, amount int64)(*protocolv2.TransactionV2,error){
	tx:=protocolv2.NewTransactionV2(TestnetChainID,protocolv2.TxHBTTransfer,n.sender,to,amount,n.seq,21000,100,nil)
	tx.Signature=""; tx.ID=""
	return tx,nil
}
func (n *fakeNode) Submit(raw string)(string,error){
	n.submitted++
	if n.fail{return "",errors.New("uncertain")}
	tx,e:=protocolv2.DecodeRawTransactionV2(raw);if e!=nil{return "",e}
	return "0x"+tx.HashHex(),nil
}
type fakeSigner struct{ w *wallet.Wallet }
func (s fakeSigner) Address()string{return s.w.Address()}
func (s fakeSigner) Sign(tx *protocolv2.TransactionV2)(*protocolv2.TransactionV2,error){return tx,tx.Sign(s.w)}

func testService(t *testing.T)(*Service,*fakeNode,string){
	t.Helper()
	dir:=t.TempDir()
	store,e:=OpenAuditStore(filepath.Join(dir,"audit.jsonl"),time.Unix(1700000000,0));if e!=nil{t.Fatal(e)}
	w,e:=wallet.NewWallet();if e!=nil{t.Fatal(e)}
	to,e:=wallet.NewWallet();if e!=nil{t.Fatal(e)}
	n:=&fakeNode{sender:w.Address()}
	s:=&Service{Config:Config{ChainID:TestnetChainID,AmountUnits:500,DailyCapUnits:1000,GlobalReserve:2000,Cooldown:time.Hour},Store:store,Node:n,Signer:fakeSigner{w},Now:func()time.Time{return time.Unix(1700000000,0)}}
	return s,n,to.Address()
}
func TestClaimIdempotentAndAppendOnly(t *testing.T){
	s,n,to:=testService(t)
	id:="0123456789abcdef"
	r,e:=s.Claim(id,to);if e!=nil{t.Fatal(e)}
	if r.Decision!=Accepted||n.submitted!=1{t.Fatalf("first claim %+v submits=%d",r,n.submitted)}
	r2,e:=s.Claim(id,to);if e!=nil{t.Fatal(e)}
	if r2.TransactionID!=r.TransactionID||n.submitted!=1{t.Fatal("idempotency created replacement submission")}
	b,e:=os.ReadFile(s.Store.path);if e!=nil{t.Fatal(e)}
	if strings.Count(strings.TrimSpace(string(b)),"\n")!=0{t.Fatalf("expected exactly one audit line: %q",b)}
}
func TestCooldownAndProtectedFailClosed(t *testing.T){
	s,n,to:=testService(t)
	if _,e:=s.Claim("0123456789abcdef",to);e!=nil{t.Fatal(e)}
	if _,e:=s.Claim("fedcba9876543210",to);e==nil||n.submitted!=1{t.Fatal("cooldown did not fail closed")}
	s2,n2,to2:=testService(t);s2.Config.Protected=[]string{to2}
	if _,e:=s2.Claim("aaaaaaaaaaaaaaaa",to2);e==nil||n2.submitted!=0{t.Fatal("protected address submitted")}
}
func TestAcceptedBeforeUncertainSubmissionIsNeverReplaced(t *testing.T){
	s,n,to:=testService(t);n.fail=true
	id:="bbbbbbbbbbbbbbbb"
	r,e:=s.Claim(id,to);if e==nil||r.Decision!=Accepted||n.submitted!=1{t.Fatal("uncertain submission not retained")}
	n.fail=false
	r2,e:=s.Claim(id,to);if e!=nil{t.Fatal(e)}
	if r2.TransactionID!=r.TransactionID||n.submitted!=1{t.Fatal("uncertain accepted request was resubmitted")}
}
func TestWrongPreparedChainRejected(t *testing.T){
	s,n,to:=testService(t)
	n.sender=to // signer mismatch
	if _,e:=s.Claim("cccccccccccccccc",to);e==nil||n.submitted!=0{t.Fatal("invalid prepared tx submitted")}
}
