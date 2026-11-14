package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hashburst/internal/faucet"
	"hashburst/wallet"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type ipLimiter struct{mu sync.Mutex;last map[string]time.Time;window time.Duration}
func(l *ipLimiter)allow(host string)bool{l.mu.Lock();defer l.mu.Unlock();now:=time.Now();if t:=l.last[host];!t.IsZero()&&now.Sub(t)<l.window{return false};l.last[host]=now;return true}
func main(){
	var listen,rpc,signer,audit,address,protected,cooldown string
	var chain,limit uint64;var amount,daily,reserve,feeCap int64
	flag.StringVar(&listen,"listen","127.0.0.1:28100","faucet API listen address")
	flag.StringVar(&rpc,"rpc","","native HVM RPC HTTPS or loopback URL")
	flag.StringVar(&signer,"signer","http://127.0.0.1:28110","dedicated signer loopback endpoint")
	flag.StringVar(&audit,"audit","","append-only audit JSONL")
	flag.StringVar(&address,"address","","dedicated faucet address")
	flag.StringVar(&protected,"protected","","comma-separated protected addresses")
	flag.StringVar(&cooldown,"cooldown","1h","recipient cooldown")
	flag.Uint64Var(&chain,"chain-id",0,"4735490 or 4735489")
	flag.Int64Var(&amount,"amount-units",0,"units per claim")
	flag.Int64Var(&daily,"daily-cap-units",0,"daily distribution cap")
	flag.Int64Var(&reserve,"global-reserve-units",0,"maximum distributable reserve")
	flag.Uint64Var(&limit,"compute-limit",21000,"native transfer compute limit")
	flag.Int64Var(&feeCap,"fee-cap-units",1000000,"maximum native fee")
	flag.Parse()
	d,e:=time.ParseDuration(cooldown);if e!=nil{panic(e)}
	var pp []string;if strings.TrimSpace(protected)!=""{for _,p:=range strings.Split(protected,","){p=strings.TrimSpace(p);if !wallet.IsValidAddress(p){panic("invalid protected address")};pp=append(pp,p)}}
	cfg:=faucet.Config{ChainID:chain,AmountUnits:amount,Cooldown:d,DailyCapUnits:daily,GlobalReserve:reserve,Protected:pp};if e=cfg.Validate();e!=nil{panic(e)}
	store,e:=faucet.OpenAuditStore(audit,time.Now());if e!=nil{panic(e)}
	sig,e:=faucet.NewRemoteSigner(signer,address,chain);if e!=nil{panic(e)}
	node,e:=faucet.NewRPCNode(rpc,chain,address,limit,feeCap);if e!=nil{panic(e)}
	svc:=&faucet.Service{Config:cfg,Store:store,Node:node,Signer:sig}
	limiter:=&ipLimiter{last:map[string]time.Time{},window:time.Minute}
	mux:=http.NewServeMux()
	mux.HandleFunc("/health",func(w http.ResponseWriter,r *http.Request){if r.Method!="GET"{http.Error(w,"method",405);return};json.NewEncoder(w).Encode(map[string]any{"status":"ok","chain_id":chain,"address":address})})
	mux.HandleFunc("/v1/claim",func(w http.ResponseWriter,r *http.Request){
		if r.Method!="POST"{http.Error(w,"method",405);return}
		host,_,e:=net.SplitHostPort(r.RemoteAddr);if e!=nil||!limiter.allow(host){http.Error(w,"rate limited",429);return}
		r.Body=http.MaxBytesReader(w,r.Body,8192)
		var q struct{RequestID string `json:"request_id"`;Recipient string `json:"recipient"`}
		dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields();if e=dec.Decode(&q);e!=nil{http.Error(w,"invalid request",400);return};var extra any;if dec.Decode(&extra)!=io.EOF{http.Error(w,"trailing JSON",400);return}
		rec,e:=svc.Claim(q.RequestID,q.Recipient);w.Header().Set("Content-Type","application/json")
		if e!=nil{w.WriteHeader(409);json.NewEncoder(w).Encode(rec);return}
		json.NewEncoder(w).Encode(rec)
	})
	s:=&http.Server{Addr:listen,Handler:mux,ReadHeaderTimeout:5*time.Second,ReadTimeout:10*time.Second,WriteTimeout:20*time.Second,IdleTimeout:30*time.Second}
	fmt.Printf("HVM_FAUCET_API_READY chain=%d address=%s listen=%s pid=%d\n",chain,address,listen,os.Getpid())
	if e=s.ListenAndServe();e!=nil&&e!=http.ErrServerClosed{panic(e)}
}
