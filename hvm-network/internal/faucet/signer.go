package faucet

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hashburst/protocolv2"
	"hashburst/wallet"
)

type RemoteSigner struct {
	endpoint string
	address string
	chain uint64
	client *http.Client
}
func NewRemoteSigner(endpoint,address string,chain uint64)(*RemoteSigner,error){
	u,e:=url.Parse(endpoint);if e!=nil{return nil,e}
	ip:=net.ParseIP(u.Hostname())
	if u.Scheme!="http"||ip==nil||!ip.IsLoopback()||u.User!=nil||u.Fragment!=""||!wallet.IsValidAddress(address){return nil,errors.New("signer must be an explicit loopback HTTP endpoint")}
	if chain!=TestnetChainID&&chain!=MainnetChainID{return nil,errors.New("unsupported signer chain")}
	return &RemoteSigner{endpoint:strings.TrimRight(endpoint,"/"),address:address,chain:chain,client:&http.Client{Timeout:15*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return errors.New("signer redirect refused")}}},nil
}
func(s *RemoteSigner)Address()string{return s.address}
func(s *RemoteSigner)Sign(tx *protocolv2.TransactionV2)(*protocolv2.TransactionV2,error){
	if tx==nil||tx.ChainID!=s.chain||tx.Type!=protocolv2.TxHBTTransfer||!wallet.AddressEqual(tx.Sender,s.address)||tx.Signature!=""{return nil,errors.New("unsigned faucet transfer does not match signer boundary")}
	b,e:=json.Marshal(tx);if e!=nil{return nil,e}
	r,e:=s.client.Post(s.endpoint+"/v1/sign","application/json",bytes.NewReader(b));if e!=nil{return nil,e};defer r.Body.Close()
	if r.StatusCode!=200{return nil,errors.New("dedicated signer rejected transaction")}
	b,e=io.ReadAll(io.LimitReader(r.Body,1<<20));if e!=nil{return nil,e};if len(b)>=1<<20{return nil,errors.New("signer response too large")}
	var out protocolv2.TransactionV2
	d:=json.NewDecoder(bytes.NewReader(b));d.DisallowUnknownFields();if e=d.Decode(&out);e!=nil{return nil,e}
	if e=out.Verify(s.chain);e!=nil{return nil,e}
	if !wallet.AddressEqual(out.Sender,s.address)||!wallet.AddressEqual(out.To,tx.To)||out.ValueUnits!=tx.ValueUnits||out.Sequence!=tx.Sequence||out.ComputeLimit!=tx.ComputeLimit||out.MaxFeeUnits!=tx.MaxFeeUnits{return nil,errors.New("signer changed transaction intent")}
	return &out,nil
}
