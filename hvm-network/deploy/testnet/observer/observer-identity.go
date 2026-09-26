// Generate only a libp2p identity; no consensus or account signing key.
package main
import (
 "crypto/rand"
 "encoding/base64"
 "flag"
 "fmt"
 "os"
 crypto "github.com/libp2p/go-libp2p/core/crypto"
 "github.com/libp2p/go-libp2p/core/peer"
)
func main(){
 path:=flag.String("key","","new private key file");flag.Parse()
 if *path=="" { panic("--key required") }
 key,_,err:=crypto.GenerateEd25519Key(rand.Reader);if err!=nil{panic(err)}
 raw,err:=crypto.MarshalPrivateKey(key);if err!=nil{panic(err)}
 id,err:=peer.IDFromPrivateKey(key);if err!=nil{panic(err)}
 f,err:=os.OpenFile(*path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if err!=nil{panic(err)}
 if _,err=f.WriteString(base64.StdEncoding.EncodeToString(raw)+"\n");err!=nil{panic(err)}
 if err=f.Sync();err!=nil{panic(err)};if err=f.Close();err!=nil{panic(err)}
 fmt.Println(id.String())
}
