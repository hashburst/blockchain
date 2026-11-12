package main

import (
  "context"
  "crypto/ecdsa"
  "encoding/hex"
  "encoding/json"
  "flag"
  "fmt"
  "github.com/ethereum/go-ethereum"
  "github.com/ethereum/go-ethereum/common"
  "github.com/ethereum/go-ethereum/core/types"
  "github.com/ethereum/go-ethereum/crypto"
  "github.com/ethereum/go-ethereum/ethclient"
  "github.com/ethereum/go-ethereum/rpc"
  "math/big"
  "os"
  "strings"
  "time"
)

const chainID int64 = 4735489
const contractInit = "600b600c600039600b6000f3602a60005560006000a000"

func key(path string) (*ecdsa.PrivateKey,error) {
  st,e:=os.Lstat(path); if e!=nil{return nil,e}
  if !st.Mode().IsRegular() || st.Mode().Perm()&0077!=0{return nil,fmt.Errorf("private key permissions invalid")}
  b,e:=os.ReadFile(path); if e!=nil{return nil,e}
  return crypto.HexToECDSA(strings.TrimSpace(string(b)))
}
func waitReceipt(ctx context.Context,c *ethclient.Client,h common.Hash)(*types.Receipt,error){
  deadline:=time.Now().Add(3*time.Minute)
  for time.Now().Before(deadline){r,e:=c.TransactionReceipt(ctx,h);if e==nil{return r,nil};if e!=ethereum.NotFound{return nil,e};time.Sleep(2*time.Second)}
  return nil,fmt.Errorf("receipt timeout %s",h)
}
func send(ctx context.Context,c *ethclient.Client,k *ecdsa.PrivateKey,to *common.Address,value *big.Int,data []byte)(*types.Receipt,error){
  from:=crypto.PubkeyToAddress(k.PublicKey)
  nonce,e:=c.PendingNonceAt(ctx,from);if e!=nil{return nil,e}
  tip,e:=c.SuggestGasTipCap(ctx);if e!=nil{tip=big.NewInt(1)}
  head,e:=c.HeaderByNumber(ctx,nil);if e!=nil{return nil,e}
  fee:=new(big.Int).Add(new(big.Int).Mul(head.BaseFee,big.NewInt(2)),tip)
  msg:=ethereum.CallMsg{From:from,To:to,Value:value,Data:data,GasTipCap:tip,GasFeeCap:fee}
  gas,e:=c.EstimateGas(ctx,msg);if e!=nil{return nil,e}
  tx:=types.NewTx(&types.DynamicFeeTx{ChainID:big.NewInt(chainID),Nonce:nonce,GasTipCap:tip,GasFeeCap:fee,Gas:gas,To:to,Value:value,Data:data})
  tx,e=types.SignTx(tx,types.LatestSignerForChainID(big.NewInt(chainID)),k);if e!=nil{return nil,e}
  if e=c.SendTransaction(ctx,tx);e!=nil{return nil,e}
  return waitReceipt(ctx,c,tx.Hash())
}
func main(){
  endpoint:=flag.String("rpc","https://blockchainapi.one/api/hashburst/hvm/mainnet/evm","public mainnet EVM RPC")
  wsURL:=flag.String("ws","wss://blockchainapi.one/api/hashburst/hvm/mainnet/evm/ws","public mainnet EVM WebSocket")
  keyPath:=flag.String("key","","miner-only funded key")
  out:=flag.String("out","/tmp/mainnet-r8c-r8d-canary.json","public evidence")
  flag.Parse()
  if *keyPath==""{panic("--key required")}
  k,e:=key(*keyPath);if e!=nil{panic(e)}
  ctx,cancel:=context.WithTimeout(context.Background(),8*time.Minute);defer cancel()
  c,e:=ethclient.DialContext(ctx,*endpoint);if e!=nil{panic(e)};defer c.Close()
  cid,e:=c.ChainID(ctx);if e!=nil||cid.Int64()!=chainID{panic("wrong mainnet chain")}
  from:=crypto.PubkeyToAddress(k.PublicKey)
  bal,e:=c.BalanceAt(ctx,from,nil);if e!=nil||bal.Sign()<=0{panic("miner key not funded by finalized APoW reward")}

  ws,e:=rpc.DialWebsocket(ctx,*wsURL,"https://blockchainapi.one");if e!=nil{panic(e)};defer ws.Close()
  r1,e:=send(ctx,c,k,&from,big.NewInt(1),nil);if e!=nil||r1.Status!=1{panic("transfer acceptance failed")}
  init, _:=hex.DecodeString(contractInit)
  rd,e:=send(ctx,c,k,nil,big.NewInt(0),init);if e!=nil||rd.Status!=1||rd.ContractAddress==(common.Address{}){panic("deploy acceptance failed")}
  logs:=make(chan types.Log,32)
  subL,e:=ws.EthSubscribe(ctx,logs,"logs",map[string]any{"address":rd.ContractAddress.Hex()});if e!=nil{panic(e)};defer subL.Unsubscribe()
  // Subscribe to newHeads immediately before the target call. Drain both
  // subscriptions while the transaction is being finalized so the exact
  // target block cannot be lost behind receipt polling.
  heads:=make(chan *types.Header,32);subH,e:=ws.EthSubscribe(ctx,heads,"newHeads");if e!=nil{panic(e)};defer subH.Unsubscribe()
  type callResult struct{ r *types.Receipt; e error }
  callDone:=make(chan callResult,1)
  go func(){r,e:=send(ctx,c,k,&rd.ContractAddress,big.NewInt(0),nil);callDone<-callResult{r:r,e:e}}()

  var rc *types.Receipt
  var bufferedHeads []*types.Header
  var bufferedLogs []types.Log
  var firstHeadAt,firstLogAt time.Time
  for rc==nil { select {
  case h:=<-heads:
    if h!=nil {
      if firstHeadAt.IsZero(){firstHeadAt=time.Now()}
      bufferedHeads=append(bufferedHeads,h)
      fmt.Printf("HVM_R8D_WS_NEWHEAD_CAPTURED_PRE_RECEIPT count=%d hash=%s\\n",len(bufferedHeads),h.Hash().Hex())
    }
  case l:=<-logs:
    if firstLogAt.IsZero(){firstLogAt=time.Now()}
    bufferedLogs=append(bufferedLogs,l)
    fmt.Printf("HVM_R8D_WS_LOG_CAPTURED_PRE_RECEIPT count=%d tx=%s block=%s\\n",len(bufferedLogs),l.TxHash.Hex(),l.BlockHash.Hex())
  case cr:=<-callDone:
    if cr.e!=nil||cr.r==nil||cr.r.Status!=1{panic("contract call acceptance failed")}
    rc=cr.r
  case e:=<-subH.Err(): if e!=nil{panic(fmt.Sprintf("newHeads subscription error: %v",e))}
  case e:=<-subL.Err(): if e!=nil{panic(fmt.Sprintf("logs subscription error: %v",e))}
  case <-ctx.Done(): panic("context expired while awaiting contract call")
  }}
  if len(rc.Logs)==0{panic("receipt log missing")}
  got,e:=c.FilterLogs(ctx,ethereum.FilterQuery{BlockHash:&rc.BlockHash,Addresses:[]common.Address{rd.ContractAddress}});if e!=nil||len(got)==0{panic("eth_getLogs acceptance failed")}

  var headOK,logOK bool
  headsSeen,logsSeen:=0,0
  checkHead:=func(h *types.Header){
    if h==nil{return};headsSeen++
    match:=h.Hash()==rc.BlockHash
    fmt.Printf("HVM_R8D_WS_NEWHEAD_SEEN count=%d hash=%s target=%s match=%t\\n",headsSeen,h.Hash().Hex(),rc.BlockHash.Hex(),match)
    if match{headOK=true}
  }
  checkLog:=func(l types.Log){
    logsSeen++
    match:=l.TxHash==rc.TxHash&&l.BlockHash==rc.BlockHash
    fmt.Printf("HVM_R8D_WS_LOG_SEEN count=%d tx=%s block=%s target_tx=%s target_block=%s match=%t\\n",logsSeen,l.TxHash.Hex(),l.BlockHash.Hex(),rc.TxHash.Hex(),rc.BlockHash.Hex(),match)
    if match{logOK=true}
  }
  for _,h:=range bufferedHeads{checkHead(h)}
  for _,l:=range bufferedLogs{checkLog(l)}
  deadline:=time.NewTimer(30*time.Second); defer deadline.Stop()
  extended:=false
  for !(headOK&&logOK){select{
  case h:=<-heads:
    if firstHeadAt.IsZero(){firstHeadAt=time.Now()}
    checkHead(h)
  case l:=<-logs:
    if firstLogAt.IsZero(){firstLogAt=time.Now()}
    checkLog(l)
  case e:=<-subH.Err(): if e!=nil{panic(fmt.Sprintf("newHeads subscription error: %v",e))}
  case e:=<-subL.Err(): if e!=nil{panic(fmt.Sprintf("logs subscription error: %v",e))}
  case <-deadline.C:
    if (headOK||logOK)&&!extended {
      extended=true
      fmt.Printf("HVM_R8D_WS_PARTIAL_EVIDENCE_EXTEND head_ok=%t log_ok=%t heads_seen=%d logs_seen=%d extra_seconds=30\\n",headOK,logOK,headsSeen,logsSeen)
      deadline.Reset(30*time.Second)
      continue
    }
    panic(fmt.Sprintf("websocket evidence timeout head_ok=%t log_ok=%t heads_seen=%d logs_seen=%d extended=%t",headOK,logOK,headsSeen,logsSeen,extended))
  }}
  firstHead:="";if !firstHeadAt.IsZero(){firstHead=firstHeadAt.UTC().Format(time.RFC3339Nano)}
  firstLog:="";if !firstLogAt.IsZero(){firstLog=firstLogAt.UTC().Format(time.RFC3339Nano)}
  ev:=map[string]any{"schema":1,"chain_id":chainID,"account":from.Hex(),"balance_before":bal.String(),"transfer_tx":r1.TxHash.Hex(),"deploy_tx":rd.TxHash.Hex(),"contract":rd.ContractAddress.Hex(),"call_tx":rc.TxHash.Hex(),"call_block":rc.BlockHash.Hex(),"logs":len(got),"websocket_newHeads":headOK,"websocket_logs":logOK,"websocket_heads_seen":headsSeen,"websocket_logs_seen":logsSeen,"websocket_first_head_at":firstHead,"websocket_first_log_at":firstLog,"websocket_extended_wait":extended,"founder_private_key_used":false,"validator_private_key_used":false}
  b,_:=json.MarshalIndent(ev,"","  ");if e=os.WriteFile(*out,append(b,'\n'),0644);e!=nil{panic(e)}
  fmt.Println("HVM_R8C_MAINNET_WALLET_TRANSACTION_RECEIPT_LOG_ACCEPTANCE_OK")
  fmt.Println("HVM_R8D_MAINNET_WEBSOCKET_SUBSCRIPTION_ACCEPTANCE_OK")
}
