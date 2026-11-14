package faucet

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hashburst/protocolv2"
	"hashburst/wallet"
)

type Decision string

const (
	Accepted Decision = "accepted"
	Rejected Decision = "rejected"
)

type AuditRecord struct {
	At             time.Time `json:"at"`
	RequestID      string    `json:"request_id"`
	Recipient      string    `json:"recipient"`
	AmountUnits    int64     `json:"amount_units"`
	Decision       Decision  `json:"decision"`
	Reason         string    `json:"reason,omitempty"`
	TransactionID  string    `json:"transaction_id,omitempty"`
}

type State struct {
	ByRequest map[string]AuditRecord
	LastByRecipient map[string]time.Time
	DailyUnits int64
	ReservedUnits int64
	Day string
}

type AuditStore struct {
	path string
	mu sync.Mutex
	state State
}

func OpenAuditStore(path string, now time.Time) (*AuditStore, error) {
	if path == "" {
		return nil, errors.New("audit path required")
	}
	dir := filepath.Dir(path)
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0022 != 0 {
		return nil, errors.New("audit directory must exist and not be group/world writable")
	}
	s := &AuditStore{path:path, state:State{ByRequest:map[string]AuditRecord{}, LastByRecipient:map[string]time.Time{}, Day:now.UTC().Format("2006-01-02")}}
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0600)
	if err != nil { return nil, err }
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1<<20)
	for sc.Scan() {
		var r AuditRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil { return nil, fmt.Errorf("corrupt faucet audit: %w", err) }
		if r.RequestID == "" || !validRecipient(r.Recipient) { return nil, errors.New("invalid faucet audit record") }
		if _, exists := s.state.ByRequest[r.RequestID]; exists { return nil, errors.New("duplicate request id in faucet audit") }
		s.state.ByRequest[r.RequestID]=r
		if r.Decision==Accepted {
			s.state.ReservedUnits += r.AmountUnits
			if r.At.UTC().Format("2006-01-02")==s.state.Day { s.state.DailyUnits += r.AmountUnits }
			k:=strings.ToLower(r.Recipient)
			if r.At.After(s.state.LastByRecipient[k]) { s.state.LastByRecipient[k]=r.At }
		}
	}
	if err:=sc.Err(); err!=nil { return nil, err }
	return s,nil
}

func (s *AuditStore) appendLocked(r AuditRecord) error {
	b,err:=json.Marshal(r); if err!=nil{return err}
	f,err:=os.OpenFile(s.path,os.O_WRONLY|os.O_APPEND|os.O_CREATE,0600); if err!=nil{return err}
	if _,err=f.Write(append(b,'\n'));err!=nil{f.Close();return err}
	if err=f.Sync();err!=nil{f.Close();return err}
	if err=f.Close();err!=nil{return err}
	return nil
}

type Preparer interface {
	Prepare(recipient string, amount int64) (*protocolv2.TransactionV2,error)
	Submit(raw string) (string,error)
}
type Signer interface {
	Address() string
	Sign(tx *protocolv2.TransactionV2) (*protocolv2.TransactionV2,error)
}

type Service struct {
	Config Config
	Store *AuditStore
	Node Preparer
	Signer Signer
	Now func() time.Time
	mu sync.Mutex
}

func (s *Service) Claim(requestID, recipient string) (AuditRecord,error) {
	s.mu.Lock(); defer s.mu.Unlock()
	if err:=s.Config.Validate();err!=nil{return AuditRecord{},err}
	if s.Store==nil||s.Node==nil||s.Signer==nil{return AuditRecord{},errors.New("faucet dependencies unavailable")}
	now:=time.Now().UTC(); if s.Now!=nil{now=s.Now().UTC()}
	if len(requestID)<16||len(requestID)>128{return AuditRecord{},errors.New("request id length invalid")}
	if !validRecipient(recipient){return s.reject(now,requestID,recipient,"invalid recipient")}
	for _,p:=range s.Config.Protected{if wallet.AddressEqual(p,recipient){return s.reject(now,requestID,recipient,"protected recipient")}}
	if old,ok:=s.Store.state.ByRequest[requestID];ok{return old,nil}
	if s.Store.state.Day!=now.Format("2006-01-02"){s.Store.state.Day=now.Format("2006-01-02");s.Store.state.DailyUnits=0}
	k:=strings.ToLower(recipient)
	if last:=s.Store.state.LastByRecipient[k];!last.IsZero()&&now.Sub(last)<s.Config.Cooldown{return s.reject(now,requestID,recipient,"cooldown active")}
	if s.Store.state.DailyUnits > s.Config.DailyCapUnits-s.Config.AmountUnits{return s.reject(now,requestID,recipient,"daily cap exhausted")}
	if s.Store.state.ReservedUnits > s.Config.GlobalReserve-s.Config.AmountUnits{return s.reject(now,requestID,recipient,"global reserve exhausted")}

	tx,err:=s.Node.Prepare(recipient,s.Config.AmountUnits);if err!=nil{return s.reject(now,requestID,recipient,"prepare failed")}
	if tx==nil||tx.ChainID!=s.Config.ChainID||tx.Type!=protocolv2.TxHBTTransfer||!wallet.AddressEqual(tx.Sender,s.Signer.Address())||!wallet.AddressEqual(tx.To,recipient)||tx.ValueUnits!=s.Config.AmountUnits||tx.Signature!="" {
		return s.reject(now,requestID,recipient,"prepared transaction violates faucet policy")
	}
	signed,err:=s.Signer.Sign(tx.Clone());if err!=nil{return s.reject(now,requestID,recipient,"signer failed")}
	if signed==nil||signed.Verify(s.Config.ChainID)!=nil||!wallet.AddressEqual(signed.Sender,s.Signer.Address())||!wallet.AddressEqual(signed.To,recipient)||signed.ValueUnits!=s.Config.AmountUnits {
		return s.reject(now,requestID,recipient,"signed transaction verification failed")
	}
	raw,err:=signed.EncodeRaw();if err!=nil{return AuditRecord{},err}

	// Persist the accepted decision and exact tx id BEFORE submission. A repeated
	// request is idempotent and never creates a replacement transaction.
	r:=AuditRecord{At:now,RequestID:requestID,Recipient:recipient,AmountUnits:s.Config.AmountUnits,Decision:Accepted,TransactionID:"0x"+signed.HashHex()}
	if err=s.Store.appendLocked(r);err!=nil{return AuditRecord{},err}
	s.Store.state.ByRequest[requestID]=r
	s.Store.state.LastByRecipient[k]=now
	s.Store.state.DailyUnits+=s.Config.AmountUnits
	s.Store.state.ReservedUnits+=s.Config.AmountUnits

	got,err:=s.Node.Submit(raw)
	if err!=nil{return r,fmt.Errorf("submission uncertain; accepted record retained: %w",err)}
	if !strings.EqualFold(strings.TrimPrefix(got,"0x"),signed.HashHex()){return r,errors.New("submission hash mismatch; accepted record retained")}
	return r,nil
}

func (s *Service) reject(now time.Time,id,recipient,reason string)(AuditRecord,error){
	r:=AuditRecord{At:now,RequestID:id,Recipient:recipient,AmountUnits:s.Config.AmountUnits,Decision:Rejected,Reason:reason}
	// Invalid addresses are not persisted because the audit schema deliberately
	// refuses malformed destinations; all valid policy rejections are durable.
	if validRecipient(recipient)&&s.Store!=nil {
		if old,ok:=s.Store.state.ByRequest[id];ok{return old,nil}
		if err:=s.Store.appendLocked(r);err!=nil{return AuditRecord{},err}
		s.Store.state.ByRequest[id]=r
	}
	return r,errors.New(reason)
}
