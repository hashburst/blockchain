package main
import(
 "crypto/rand";"flag";"fmt";"github.com/libp2p/go-libp2p/core/crypto";"github.com/libp2p/go-libp2p/core/peer";"os"
)
func main(){
 p:=flag.String("key","","observer p2p private key");flag.Parse();if *p==""{panic("--key required")}
 var k crypto.PrivKey
 if b,e:=os.ReadFile(*p);e==nil{st,e:=os.Lstat(*p);if e!=nil||!st.Mode().IsRegular()||st.Mode().Perm()&0077!=0{panic("unsafe key file")};k,e=crypto.UnmarshalPrivateKey(b);if e!=nil{panic(e)}
 }else if os.IsNotExist(e){var e error;k,_,e=crypto.GenerateEd25519Key(rand.Reader);if e!=nil{panic(e)};b,e:=crypto.MarshalPrivateKey(k);if e!=nil{panic(e)};f,e:=os.OpenFile(*p,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if e!=nil{panic(e)};if _,e=f.Write(b);e!=nil{panic(e)};if e=f.Sync();e!=nil{panic(e)};if e=f.Close();e!=nil{panic(e)}
 }else{panic(e)}
 id,e:=peer.IDFromPrivateKey(k);if e!=nil{panic(e)};fmt.Println(id.String())
}
