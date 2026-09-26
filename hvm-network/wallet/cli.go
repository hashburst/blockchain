package wallet

// cli.go — comandi wallet, invocati da main.go come:
//     hashburst-node wallet <comando> [opzioni]
//
// LA PASSWORD NON SI PASSA COME ARGOMENTO. Gli argomenti di un processo sono
// leggibili da chiunque sulla macchina (`ps aux`, /proc/<pid>/cmdline) e
// finiscono nella cronologia della shell. Si passa un file, o la variabile
// HB_WALLET_PASSWORD.

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
)

const cliUsage = `hashburst-node wallet — gestione wallet HashBurst (compatibile EVM)

  wallet new       --dir <keystore-dir> --password-file <file>
                   Genera un wallet nuovo e lo salva cifrato.

  wallet address   --keystore <file> --password-file <file>
                   Stampa l'indirizzo di un keystore.

  wallet import    --key <file-con-chiave-hex> --dir <dir> --password-file <file>
                   Importa una chiave privata esistente in un keystore.

  wallet verify    --address <0x...>
                   Verifica forma e checksum EIP-55 di un indirizzo.

La password si legge da --password-file oppure da HB_WALLET_PASSWORD.
Il keystore e' nel formato Web3 v3: importabile in MetaMask e geth.
`

func readPassword(file string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("lettura password: %w", err)
		}
		p := strings.TrimRight(string(b), "\r\n")
		if p == "" {
			return "", fmt.Errorf("%s e' vuoto", file)
		}
		return p, nil
	}
	if p := os.Getenv("HB_WALLET_PASSWORD"); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("password assente: usa --password-file o HB_WALLET_PASSWORD")
}

// RunCLI esegue un comando wallet. Ritorna il codice di uscita.
func RunCLI(args []string) int {
	if len(args) < 1 {
		fmt.Print(cliUsage)
		return 1
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "new":
		fs := flag.NewFlagSet("wallet new", flag.ContinueOnError)
		dir := fs.String("dir", "./keystore", "directory del keystore")
		pwFile := fs.String("password-file", "", "file contenente la password")
		light := fs.Bool("light", false, "parametri scrypt leggeri (piu' veloce, meno robusto)")
		if err := fs.Parse(rest); err != nil {
			return 1
		}
		password, err := readPassword(*pwFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		w, err := NewWallet()
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		n, r, p := StandardScryptN, StandardScryptR, StandardScryptP
		if *light {
			n, r, p = LightScryptN, LightScryptR, LightScryptP
		}
		path, err := w.SaveKeystore(*dir, password, n, r, p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		fmt.Printf("Indirizzo: %s\n", w.Address())
		fmt.Printf("Keystore:  %s\n", path)
		fmt.Println()
		fmt.Println("Metti l'indirizzo in /etc/hashburst/env come REWARD_ADDRESS.")
		fmt.Println("Il keystore e la password restano tuoi: il nodo non ne ha bisogno,")
		fmt.Println("per ricevere le reward gli basta l'indirizzo.")
		fmt.Println("Se perdi il file o la password, i fondi non sono recuperabili.")
		return 0

	case "address":
		fs := flag.NewFlagSet("wallet address", flag.ContinueOnError)
		ks := fs.String("keystore", "", "file keystore")
		pwFile := fs.String("password-file", "", "file contenente la password")
		if err := fs.Parse(rest); err != nil {
			return 1
		}
		if *ks == "" {
			fmt.Fprintln(os.Stderr, "errore: --keystore obbligatorio")
			return 1
		}
		password, err := readPassword(*pwFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		w, err := LoadKeystore(*ks, password)
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		fmt.Println(w.Address())
		return 0

	case "import":
		fs := flag.NewFlagSet("wallet import", flag.ContinueOnError)
		keyFile := fs.String("key", "", "file con la chiave privata esadecimale")
		dir := fs.String("dir", "./keystore", "directory del keystore")
		pwFile := fs.String("password-file", "", "file contenente la password")
		light := fs.Bool("light", false, "parametri scrypt leggeri")
		if err := fs.Parse(rest); err != nil {
			return 1
		}
		if *keyFile == "" {
			fmt.Fprintln(os.Stderr, "errore: --key obbligatorio")
			return 1
		}
		raw, err := os.ReadFile(*keyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		w, err := FromPrivateKeyHex(string(raw))
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		password, err := readPassword(*pwFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		n, r, p := StandardScryptN, StandardScryptR, StandardScryptP
		if *light {
			n, r, p = LightScryptN, LightScryptR, LightScryptP
		}
		path, err := w.SaveKeystore(*dir, password, n, r, p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "errore:", err)
			return 1
		}
		fmt.Printf("Indirizzo: %s\n", w.Address())
		fmt.Printf("Keystore:  %s\n", path)
		fmt.Fprintln(os.Stderr, "\nAvviso: la chiave in chiaro e' ancora in "+*keyFile+". Cancellala.")
		return 0

	case "verify":
		fs := flag.NewFlagSet("wallet verify", flag.ContinueOnError)
		addr := fs.String("address", "", "indirizzo 0x da verificare")
		if err := fs.Parse(rest); err != nil {
			return 1
		}
		if !IsValidAddress(*addr) {
			fmt.Printf("NON VALIDO: %s\n", *addr)
			return 1
		}
		body := strings.TrimPrefix(*addr, "0x")
		b, _ := hex.DecodeString(body)
		checksummed := ToChecksumAddress(b)
		if body == strings.ToLower(body) || body == strings.ToUpper(body) {
			fmt.Printf("Valido (senza checksum). Forma con checksum EIP-55:\n%s\n", checksummed)
		} else {
			fmt.Printf("Valido, checksum EIP-55 corretto:\n%s\n", checksummed)
		}
		return 0

	case "help", "-h", "--help":
		fmt.Print(cliUsage)
		return 0
	}

	fmt.Fprintf(os.Stderr, "comando sconosciuto: %s\n\n", cmd)
	fmt.Fprint(os.Stderr, cliUsage)
	return 1
}
