# Recupero EVM della testnet HashBurst

Questo pacchetto riprende il rollout interrotto con il piano originale: chain ID 4735490, attivazione EVM a 53303, gas per blocco 200000, base fee 1 wei. Non eseguire il vecchio activate.py/rollout.py. Non rigenerare il piano, il genesis, le identità o i journal. La rete legacy 1337 resta invariata; questo pacchetto non attiva mainnet 4735489.

Il runtime corregge la lettura di un recovery snapshot creato prima dell'aggiunta della futura attivazione EVM. La compatibilità del digest è ammessa esclusivamente prima dell'altezza di attivazione sulla testnet 4735490, quando tutte le altre regole coincidono. Lock, QC, firme, parent e journal continuano a essere verificati. Snapshot e journal non vengono riscritti dalla migrazione.

I test locali sono descritti in TEST_RESULT.txt. Non certificano l'esito sulle VPS. Il timeout originale non è stato riprodotto sul database live: i nuovi log espongono verifica, replay, recovery e PID/CPU. Un processo realmente bloccato produrrà una diagnosi; il pacchetto non lo aggira cancellando i dati.

## Procedura in sequenza

Dopo estrazione e checksum, `python3 finish.py --plan ../HashBurst-EVM-Testnet-RC-19a76ac5/plan.json` esegue recupero, riavvio di v4 e pubblicazione HTTPS in ordine, fermandosi al primo errore. Dopo il successo restano le conferme reali MetaMask (sezione 4) e la revisione/merge GitHub (sezione 5). Le sezioni seguenti consentono anche di eseguire e riprendere ogni fase separatamente.

## 1. Sul Mac: recupero e avvio

Richiede Python 3.10+, SSH/scp e password dei cinque nodi. I binari sono Linux amd64 e vengono eseguiti solo sulle VPS.

```sh
tar -xzf HashBurst-EVM-Recovery-v1.0.0.tar.gz
cd HashBurst-EVM-Recovery-v1.0.0
shasum -a 256 -c SHA256SUMS &&
python3 recover.py recover --plan ../HashBurst-EVM-Testnet-RC-19a76ac5/plan.json
```

Il comando verifica il piano originale e i journal salvati dal precedente stop, trasferisce il nuovo runtime, esegue migrazione e verifica offline sui cinque nodi, poi avvia i servizi solo dopo cinque esiti positivi. Non aggiorna altri servizi. Per ogni nodo la migrazione è un job systemd indipendente dalla sessione SSH. Il polling dura al massimo quattro ore: la sua scadenza lascia il job intatto.

Dopo un'interruzione SSH o del polling, lo stesso comando si riaggancia ai job. Non riavviarli manualmente. Un job fallito richiede l'esame della diagnosi stampata; `--retry-offline` consente un tentativo solo se i servizi restano fermi e i file protetti corrispondono al precedente tentativo. Non usare questo flag per superare una verifica fallita.

Se i cinque job sono completati ma l'avvio dei servizi è stato interrotto:

```sh
python3 recover.py start-prepared --plan ../HashBurst-EVM-Testnet-RC-19a76ac5/plan.json
```

`start-prepared` richiede cinque risultati offline coerenti e avvia soltanto servizi non già attivi. Non migra e non riavvia quelli attivi.

Per sola verifica dopo l'avvio:

```sh
python3 recover.py verify --plan ../HashBurst-EVM-Testnet-RC-19a76ac5/plan.json
```

Il confronto usa una stessa altezza finalizzata sui quattro validatori e sull'observer e include i commitment EVM dopo l'attivazione. Le attestazioni sono verificate dal runtime: il comparatore non è un secondo verificatore crittografico indipendente.

## 2. Un solo riavvio controllato

```sh
python3 recover.py restart-v4 --plan ../HashBurst-EVM-Testnet-RC-19a76ac5/plan.json
```

Gli altri tre validatori rimangono attivi. Il comando richiede avanzamento, accordo finale, prefisso journal e pin conservati e una nuova registrazione PRECOMMIT. Se esiste la prova del riavvio, riprende la verifica senza richiedere un secondo riavvio. Se il riavvio è stato interrotto fra stop e start, usare prima `start-prepared` e poi ripetere il controllo. Il replay può richiedere tempo.

## 3. Pubblicazione dell'ingress EVM

```sh
python3 publish.py
```

Richiede GATE-restart-v4.json per il binario distribuito. Installa il gateway e le route dedicate su 64.31.4.9, preservando l'ingress HVM nativo, poi verifica HTTPS dal Mac. Non espone API amministrative, chiavi o funzioni di firma del nodo.

- JSON-RPC: https://blockchainapi.one/api/hashburst/hvm/testnet/evm
- WebSocket: wss://blockchainapi.one/api/hashburst/hvm/testnet/evm/ws
- Collaudo: https://blockchainapi.one/hvm-testnet-metamask/

L'installer è destinato alla prima pubblicazione. Se rileva un'installazione EVM già presente, si ferma senza sovrascriverla. Dopo una pubblicazione riuscita basta `python3 verify-public.py`. Un'installazione parziale richiede di leggere il backup/log indicato, non cancellare file alla cieca.

## 4. Account MetaMask e tre transazioni reali

Aprire la pagina di collaudo in un browser con MetaMask. Collegare un account dedicato alla testnet. Annotarne l'indirizzo pubblico. Nessuna seed phrase o chiave deve essere incollata nella pagina o trasferita.

Dal Mac, sostituire il segnaposto con l'indirizzo copiato da MetaMask:

```sh
scp hvm-evm-fund root@77.90.188.153:/root/hvm-evm-fund
ssh root@77.90.188.153
chmod 700 /root/hvm-evm-fund
/root/hvm-evm-fund --to 0xINDIRIZZO_METAMASK
exit
```

La utility invia esattamente 1 tHBT dall'operatore testnet già provisionato. Controlla chain ID e identità; la chiave viene letta solo localmente. Salva la transazione firmata prima dell'invio. Ripetere lo stesso comando riprende quella transazione; non crea un secondo versamento per lo stesso destinatario. Un timeout non autorizza a eliminare il file di stato.

Ricollegare MetaMask nella pagina, premere Esegui e confermare nel wallet trasferimento a sé stessi, deploy e chiamata. Il collaudo verifica receipt, gas, storage 42, log, notifiche newHeads/logs e unsubscribe. Salvare hvm-metamask-public-proof.json in questa cartella.

```sh
python3 verify-public.py --wallet-proof hvm-metamask-public-proof.json
python3 closeout.py
```

La verifica confronta il report del browser con le receipt e i log canonici pubblici. La provenienza MetaMask resta una prova manuale dell'operatore, non un'attestazione remota del browser. I risultati coprono questi scenari e la versione MetaMask usata, non ogni API Ethereum. Annotare versione browser/MetaMask nella revisione PR.

## 5. GitHub e merge

SOURCE_COMMIT identifica il commit del runtime e degli strumenti aggiunti alla PR 19. Il pacchetto non modifica master senza revisione. Con GitHub CLI autenticata sul Mac:

```sh
python3 closeout.py --github
gh pr view 19 --repo hashburst/blockchain
gh pr checks 19 --repo hashburst/blockchain
```

La prima istruzione allega alla PR le prove reali, senza chiavi o journal completi. Per il merge, dopo verifica dei risultati, revisione delle regole di consenso/economia e tutti i controlli richiesti:

```sh
gh pr ready 19 --repo hashburst/blockchain
gh pr merge 19 --repo hashburst/blockchain --merge --match-head-commit "$(cat SOURCE_COMMIT)"
```

Il vincolo al commit impedisce di approvare implicitamente aggiornamenti successivi. Le protezioni GitHub restano applicate. Se GitHub rifiuta un controllo o una revisione mancante, completare quel requisito senza forzare il merge.

## Mainnet separata

Dopo la chiusura testnet serviranno configurazione, distribuzione iniziale, validator set, checkpoint/genesis mainnet, parametri economici e piano di attivazione separati per 4735489, con prove e revisione proprie. Non derivare una mainnet modificando il chain ID dei database testnet e non modificare il genesis della rete legacy. Questa release non contiene un'attivazione mainnet automatica.

## Diagnostica e riproducibilità

Gli output sono conservati nelle directory evm-recovery-* sul Mac e nei job hvm-evm-repair-* su ciascuna VPS. Ogni fase fallisce mantenendo dati e prove; nessun rollback automatico dei database o journal.

Per riprodurre i test dal repository al commit SOURCE_COMMIT, usare Go 1.25.7:

```sh
cd hvm-network
CGO_ENABLED=0 go test ./... -count=1 -timeout=10m
python3 deploy/testnet/evm/recovery/test-recovery.py
cd ../evm/execution
CGO_ENABLED=0 go test ./... -count=1 -timeout=10m
```

`build-package.py` nel repository ricostruisce il pacchetto con i binari statici, i sorgenti della correzione e i checksum. Non include database, configurazioni private o chiavi.
