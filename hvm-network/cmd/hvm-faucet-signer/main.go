package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hashburst/protocolv2"
	"hashburst/wallet"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func privateFile(path string)([]byte,error){
	st,e:=os.Lstat(path);if e!=nil{return nil,e}
	if !st.Mode().IsRegular()||st.Mode().Perm()&0077!=0||st.Size()>65536{return nil,errors.New("owner-only regular secret file required")}
	return os.ReadFile(path)
}
func main(){
	var listen,keyfile,passwordfile,address string;var chain,computeLimit uint64;var maxAmount,maxFee int64
	flag.StringVar(&listen,"listen","127.0.0.1:28110","loopback signer listen address")
	flag.StringVar(&keyfile,"keystore","","encrypted faucet keystore")
	flag.StringVar(&passwordfile,"password-file","","owner-only password file")
	flag.StringVar(&address,"address","","expected faucet address")
	flag.Uint64Var(&chain,"chain-id",0,"4735490 or 4735489")\n\tflag.Int64Var(&maxAmount,"max-amount-units",0,"maximum value signer may authorize")\n\tflag.Int64Var(&maxFee,"max-fee-units",0,"maximum fee signer may authorize")\n\tflag.Uint64Var(&computeLimit,"compute-limit",21000,"exact transfer compute limit")
	flag.Parse()
	host,_,e:=net.SplitHostPort(listen);if e!=nil||!net.ParseIP(host).IsLoopback(){panic("signer listen must be explicit loopback")}
	if !wallet.IsValidAddress(address)||(chain!=4735490&&chain!=4735489)||maxAmount<=0||maxFee<0||computeLimit==0{panic("explicit faucet address, chain and signing bounds required")}
	ks,e:=privateFile(keyfile);if e!=nil{panic(e)};pw,e:=privateFile(passwordfile);if e!=nil{panic(e)}
	w,e:=wallet.DecryptV3(ks,strings.TrimSuffix(string(pw),"\n"));if e!=nil{panic(e)}
	if !wallet.AddressEqual(w.Address(),address){panic("keystore does not match expected faucet address")}
	mux:=http.NewServeMux()
	mux.HandleFunc("/health",func(rw http.ResponseWriter,r *http.Request){if r.Method!="GET"{http.Error(rw,"method",405);return};json.NewEncoder(rw).Encode(map[string]any{"status":"ok","chain_id":chain,"address":w.Address()})})
	mux.HandleFunc("/v1/sign",func(rw http.ResponseWriter,r *http.Request){
		if r.Method!="POST"{http.Error(rw,"method",405);return}
		r.Body=http.MaxBytesReader(rw,r.Body,1<<20)
		var tx protocolv2.TransactionV2;d:=json.NewDecoder(r.Body);d.DisallowUnknownFields()
		if e:=d.Decode(&tx);e!=nil{http.Error(rw,"invalid transaction",400);return};var extra any;if d.Decode(&extra)!=io.EOF{http.Error(rw,"trailing JSON",400);return}
		if tx.ChainID!=chain||tx.Type!=protocolv2.TxHBTTransfer||!wallet.AddressEqual(tx.Sender,w.Address())||!wallet.IsValidAddress(tx.To)||strings.EqualFold(tx.To,"0x0000000000000000000000000000000000000000")||tx.ValueUnits<=0||tx.ValueUnits>maxAmount||tx.ComputeLimit!=computeLimit||tx.MaxFeeUnits<0||tx.MaxFeeUnits>maxFee||len(tx.Data)!=0||tx.Signature!=""{http.Error(rw,"intent rejected",400);return}
		if e:=tx.Sign(w);e!=nil{http.Error(rw,"sign failed",500);return}
		rw.Header().Set("Content-Type","application/json");json.NewEncoder(rw).Encode(&tx)
	})
	s:=&http.Server{Addr:listen,Handler:mux,ReadHeaderTimeout:5*time.Second,ReadTimeout:10*time.Second,WriteTimeout:10*time.Second,IdleTimeout:30*time.Second}
	fmt.Printf("HVM_FAUCET_SIGNER_READY chain=%d address=%s listen=%s\n",chain,w.Address(),listen)
	if e:=s.ListenAndServe();e!=nil&&e!=http.ErrServerClosed{panic(e)}
}
