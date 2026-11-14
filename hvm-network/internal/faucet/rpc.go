package faucet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hashburst/protocolv2"
	"hashburst/wallet"
)

type RPCNode struct {
	Endpoint string
	ChainID uint64
	Sender string
	ComputeLimit uint64
	FeeCapUnits int64
	client *http.Client
}

func NewRPCNode(endpoint string, chain uint64, sender string, limit uint64, feeCap int64)(*RPCNode,error){
	u,e:=url.Parse(endpoint);if e!=nil{return nil,e}
	ip:=net.ParseIP(u.Hostname())
	if u.User!=nil||u.Fragment!=""||u.Host==""||(u.Scheme!="https"&&(u.Scheme!="http"||ip==nil||!ip.IsLoopback())){return nil,errors.New("RPC requires HTTPS or explicit loopback HTTP")}
	if (chain!=TestnetChainID&&chain!=MainnetChainID)||!wallet.IsValidAddress(sender)||limit==0||feeCap<0{return nil,errors.New("invalid RPC faucet configuration")}
	n:=&RPCNode{Endpoint:endpoint,ChainID:chain,Sender:sender,ComputeLimit:limit,FeeCapUnits:feeCap,client:&http.Client{Timeout:15*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return errors.New("RPC redirect refused")}}}
	var s string;if e=n.rpc("eth_chainId",[]any{},&s);e!=nil{return nil,e}
	v,e:=hexQuantity(s);if e!=nil||v!=chain{return nil,errors.New("RPC chain mismatch")}
	return n,nil
}
func (n *RPCNode) rpc(method string,params []any,out any)error{
	b,e:=json.Marshal(map[string]any{"jsonrpc":"2.0","id":1,"method":method,"params":params});if e!=nil{return e}
	r,e:=n.client.Post(n.Endpoint,"application/json",bytes.NewReader(b));if e!=nil{return e};defer r.Body.Close()
	if r.StatusCode!=200{return fmt.Errorf("RPC HTTP %d",r.StatusCode)}
	b,e=io.ReadAll(io.LimitReader(r.Body,2097153));if e!=nil{return e};if len(b)>2097152{return errors.New("RPC response too large")}
	var v struct{JSONRPC string `json:"jsonrpc"`;ID int `json:"id"`;Result json.RawMessage `json:"result"`;Error json.RawMessage `json:"error"`}
	if e=json.Unmarshal(b,&v);e!=nil{return e};if v.JSONRPC!="2.0"||v.ID!=1{return errors.New("RPC response identity mismatch")}
	if len(v.Error)>0&&string(v.Error)!="null"{return fmt.Errorf("RPC error: %s",v.Error)}
	if len(v.Result)==0{return errors.New("RPC result missing")}
	return json.Unmarshal(v.Result,out)
}
func hexQuantity(s string)(uint64,error){if !strings.HasPrefix(s,"0x")||len(s)<3{return 0,errors.New("invalid hex quantity")};return strconv.ParseUint(s[2:],16,64)}
func (n *RPCNode) Prepare(to string,amount int64)(*protocolv2.TransactionV2,error){
	var cfg struct{ChainID uint64 `json:"chain_id"`;FeePolicy *protocolv2.FeePolicy `json:"fee_policy"`}
	if e:=n.rpc("hb_feePolicy",[]any{},&cfg);e!=nil{return nil,e}
	if cfg.ChainID!=n.ChainID||cfg.FeePolicy==nil{return nil,errors.New("native fee policy missing or wrong chain")}
	fee,e:=cfg.FeePolicy.MaxFeeForLimit(n.ComputeLimit);if e!=nil{return nil,e};if fee>n.FeeCapUnits{return nil,errors.New("node fee estimate exceeds faucet fee cap")}
	var q string;if e=n.rpc("hb_getTransactionCount",[]any{n.Sender,"pending"},&q);e!=nil{return nil,e}
	seq,e:=hexQuantity(q);if e!=nil{return nil,e}
	tx:=protocolv2.NewTransactionV2(n.ChainID,protocolv2.TxHBTTransfer,n.Sender,to,amount,seq,n.ComputeLimit,fee,nil)
	tx.Signature="";tx.ID=""
	return tx,nil
}
func (n *RPCNode) Submit(raw string)(string,error){var out string;return out,n.rpc("hb_sendRawTransactionV2",[]any{raw},&out)}
