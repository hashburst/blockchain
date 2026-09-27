# Riparazione EXEC e completamento testnet, v1.0.3

Causa identificata negli allegati del 27 settembre: tutti i cinque job offline hanno completato la migrazione, ma il pacchetto recovery creava la directory del binario root:root 0700. Il servizio utilizza un utente dedicato: systemd fallisce con 203/EXEC e Permission denied prima del replay. È un difetto dell'installer recovery.

La release mantiene gli stessi tre binari della v1.0.0/v1.0.1. Cambiano installer, ripresa e test. Nessuna modifica al consenso in questa correzione. Il runtime SHA256 è d91ed8eeef0fb9290f74516883d536caf52500878ef89099a81f1a4b6064c8d3.

La v1.0.3 corregge la ripresa parziale della v1.0.2: dopo lo stop non richiede reset-failed a un servizio già inattivo. Identifica il processo con una finestra di lettura limitata a 15 secondi, perché MainPID può essere pubblicato prima dell'esecuzione del binario. Se l'identità resta diversa o incerta, si ferma senza toccare quel processo e registra PID, hash osservato e stato. La causa specifica del messaggio di v2 non è confermata dai soli log: il controllo la distingue sul nodo.

Su v1, v3 e v4 il precedente output indica che chmod e la prova --help erano già superati prima dell'errore reset-failed. La nuova versione riprende dai marker salvati. L'observer, se già avviato, rimane in esecuzione. Non riesegue i job offline.

## Comando dal Mac

Eseguire dalla cartella che contiene il vecchio piano, dopo aver scaricato il nuovo archivio:

```sh
tar -xzf HashBurst-EVM-Recovery-v1.0.3.tar.gz
cd HashBurst-EVM-Recovery-v1.0.3
shasum -a 256 -c SHA256SUMS &&
python3 resume-live.py --plan ../HashBurst-EVM-Testnet-RC-19a76ac5/plan.json
```

Non eseguire nuovamente activate.py, finish.py o recover.py recover. Non cambiare il piano. Lasciare aperto il Mac e la connessione durante l'attesa.

## Sequenza automatica

1. Autentica cinque sessioni SSH nuove. Verifica identità, testnet 4735490, configurazione EVM esistente, binario, cinque risultati offline, prefissi dei journal, override systemd e utente del servizio. Nessuna modifica se un preflight fallisce.
2. Per il solo errore 203/EXEC, ferma il ciclo di riavvio, registra le evidenze e cambia esclusivamente la directory evm-repair-d91ed8eeef0f da 0700 a 0755. Nessun chmod ricorsivo; candidate.json, job.json e offline.py rimangono 0600. Non cambia unità, utente, hardening, database, config, pin, chiavi o journal.
3. Prova l'esecuzione di --help con UID/GID del servizio: non carica configurazioni o dati. Avvia il servizio e verifica un processo reale con lo SHA atteso. Un processo già in esecuzione con lo stesso binario viene mantenuto senza restart. Una ripresa dopo interruzione usa la prova registrata.
4. Attende il replay e confronta i cinque nodi alla stessa altezza finalizzata, includendo EVM dopo 53303. Attende cinque ulteriori blocchi. Può durare fino a quattro ore; un nuovo fallimento systemd o una sessione SSH definitivamente persa interrompe subito il controllo. Lo script non scambia un processo attivo per una finalità verificata.
5. Esegue o riprende la prova di riavvio del solo v4, controllando journal, pin e nuovo precommit. Pubblica l'ingress EVM solo dopo questi gate.

L'attivazione resta 53303, gas limit 200000, base fee 1 wei. Legacy 1337 invariata; mainnet 4735489 non viene attivata da questo pacchetto. Non vengono rigenerati checkpoint o genesis.

Se SSH si interrompe, i servizi rimangono intatti. Lo stesso comando riprende senza migrazione e senza riavviare i processi già attivi. Con un errore diverso da 203/EXEC si ferma conservando i log: non forza l'avvio. Il riavvio di v4 già registrato non viene ripetuto. La directory evm-recovery-* contiene gli esiti; inspect-live.py raccoglie lo stato in sola lettura se necessario.

La pubblicazione richiede GATE-restart-v4.json. Dopo una pubblicazione riuscita basta verify-public.py. Una pubblicazione parziale si ferma richiedendo l'esame dei suoi log.

Endpoint previsti dopo i gate:
- JSON-RPC: https://blockchainapi.one/api/hashburst/hvm/testnet/evm
- WebSocket: wss://blockchainapi.one/api/hashburst/hvm/testnet/evm/ws
- Collaudo wallet: https://blockchainapi.one/hvm-testnet-metamask/

Le conferme MetaMask rimangono manuali. Non viene dichiarato un risultato live usando soltanto i test locali.

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
python3 deploy/testnet/evm/recovery/test-exec-permissions.py
cd ../evm/execution
CGO_ENABLED=0 go test ./... -count=1 -timeout=10m
```

`build-package.py` nel repository ricostruisce il pacchetto con i binari statici, i sorgenti della correzione e i checksum. Non include database, configurazioni private o chiavi.
