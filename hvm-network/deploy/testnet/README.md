# Runtime testnet persistente — integrazione, non attivazione pubblica

Entry point: `cmd/hashburst-testnet`. Usa il modulo Go `hvm-network/` e Go 1.25.7.
Non richiama fixture, non crea genesis, non avvia mining legacy, non installa servizi.
I default del runtime legacy e i chain ID pubblici non sono modificati.

## Preparazione del binario

Dalla directory `hvm-network/` del commit revisionato:

```bash
export GOTOOLCHAIN=local
go test ./... -count=1
go test -race ./internal/testnet ./blockchain ./consensus -count=1
HB_TESTNET_INTEGRATION=1 go test ./internal/testnet -run TestFourPersistentValidatorProcesses -count=1 -v
go build -trimpath -o /tmp/hashburst-testnet ./cmd/hashburst-testnet
sha256sum /tmp/hashburst-testnet
```

Il test d'integrazione genera una fixture esclusivamente in una directory temporanea,
avvia quattro processi validatori, attende finalità, ferma un nodo e lo riavvia come
observer con la stessa identità e lo stesso disco. Richiede nuova finalità dopo il
riavvio e verifica che il journal dell'observer non sia cambiato. Poi riabilita il
ruolo validator, termina il processo con SIGKILL e verifica nuovi precommit dopo
il riavvio sullo stesso disco. Le chiavi sono
create solo dalla fixture temporanea di test e non sono chiavi di deploy.
Non eseguire questi test sulla directory di stato di un servizio.

## Configurazione e provisioning esplicito

`node.example.json` è deliberatamente NON avviabile: chain ID 0, hash vuoti e
bootnodes vuoti devono essere sostituiti con parametri di una testnet approvata.
Non assegna gli ID HVM Testnet/Mainnet. I timeout `protocol.consensus_network`
sono durate Go espresse come interi in nanosecondi nel JSON, non millisecondi.
Le fee e le soglie della configurazione esempio non sono parametri economici approvati.

Serve uno snapshot testnet coerente, preparato separatamente, con `blockchain.dat`
e `blockchain.idx`, genesis e checkpoint attesi, validator set e parametri di
protocollo esatti. La preparazione/distribuzione dello snapshot non è automatizzata
qui: non copiare il database della rete pubblica per trasformarlo in testnet.
La configurazione completa viene fornita come file, senza override d'ambiente.
Il parser rifiuta proprietà sconosciute e JSON aggiuntivo.

Per ogni nodo configurare:
- directory assoluta canonica di stato separata, per esempio `/var/lib/hashburst-hvm-testnet`;
- `node_id`, `peer_id`, chiave P2P in file base64 libp2p (permessi 0600);
- `role`: `observer` o `validator`;
- per validator: `validator_id` registrato e chiave consenso esadecimale privata in file 0600;
- per observer: `consensus_key_file` vuoto; mantenere `validator_id` se è la stessa identità precedentemente validatrice;
- genesis, checkpoint altezza/hash e configurazione di consenso condivisa;
- RPC loopback con porta dedicata e P2P su indirizzo/porta espliciti, bootnodes con peer ID.

Chiavi in chiaro su file privati sono supportate in questa integrazione interna;
keystore cifrato/KMS richiede integrazione successiva. Non inserirle nel repository.
Il lock è locale alla directory: non autorizza a duplicare una chiave su altri host.

Su snapshot ancora fermo esattamente al checkpoint configurato:

```bash
/opt/hashburst-hvm-testnet/hashburst-testnet --config /etc/hashburst-hvm-testnet/node.json --provision
/opt/hashburst-hvm-testnet/hashburst-testnet --config /etc/hashburst-hvm-testnet/node.json --check
```

`--provision` verifica catena e identità, inizializza solo i journal assenti e scrive
il pin con creazione esclusiva e fsync. Non scrive blocchi o genesis e rifiuta un
pin già presente. Un errore a metà provisioning lascia eventuali file creati per
ispezione: nessuna cancellazione automatica.
`--check` verifica senza avviare rete o firme; apre/crea il solo file di lock.
Tutti gli avvii successivi richiedono pin e journal presenti. Configurazione,
checkpoint, genesis, identità P2P, node ID e validator ID sono vincolati dal pin;
porte/bootnodes e passaggio validator→observer restano configurabili.

La unità systemd inclusa è un template, non è stata installata. Richiede utente
`hashburst-hvm-testnet`, binario e configurazione già preparati e snapshot con
ownership adeguata. Nessun script abilita automaticamente il servizio.

## Persistenza e arresto

L'apertura rigorosa controlla indice/file, hash, configurazione, verifica catena,
replay degli state root e journal. File mancanti/corrotti causano errore: nessuna
ripartenza da genesis. Per questa apertura soltanto, le scritture dei blocchi
eseguono fsync sul file dati prima di fsync dell'indice. Un crash tra i due lascia
uno stato incompleto che viene rifiutato: non c'è riparazione automatica del WAL.
SIGTERM/SIGINT cancella i loop, chiude listener/host e attende il reactor prima
di rilasciare il lock. Arresto forzato rimane soggetto ai controlli al riavvio.

Il runtime persistente scrive `consensus-recovery.json` prima delle firme: altezza,
round riservato, lock, valore valido, blocchi e certificati prevote. La scrittura usa
file temporaneo privato, fsync, rename atomico e fsync della directory. Il journal
resta append-only. Al riavvio controlla identità/configurazione, parent hash, checksum,
certificati, blocchi rieseguiti e coerenza con tutte le firme ancora pendenti.
Riprende con i lock recuperati e un round-change firmato verso un round nuovo.
Nessun salto di round autorizza a dimenticare un lock.

Snapshot mancante con firme pendenti, corrotto, precedente a un precommit, incoerente
con la catena o round esauriti bloccano il validatore. Errori di scrittura del recovery
state o del journal fermano le firme fino a riavvio e verifica. Il checksum rileva
corruzione accidentale: non autentica un disco compromesso. Servono storage affidabile
che rispetti fsync, custodia delle chiavi e una sola istanza per identità.
Non ripristinare snapshot, catena e journal da backup di epoche diverse; non cancellare
mai un journal per consentire la ripartenza.

Un journal creato dalla versione precedente senza recovery snapshot non contiene
abbastanza informazioni per ricostruire i lock a un'altezza pendente. In quel caso
rimane la procedura observer: stessa identità, nessuna chiave consenso, sincronizzazione
oltre tutte le altezze firmate, stop, `--check` come validator. Richiede che il resto
del quorum possa finalizzare; non inventa uno stato per sbloccare una rete interamente
ferma su dati precedenti. File catena/indice incompleti richiedono ancora riparazione
operativa separata: il runtime li rifiuta e non tronca dati automaticamente.

Le prove specifiche e i comandi riproducibili sono in
`review/validator-recovery/README.md`. Includono lock/QC, riavvio completo a quattro
validatori e SIGKILL di un processo con successiva ripresa delle firme.

## HTTP e limiti di rilascio

Solo `/health` GET e `/rpc`, su loopback. `/control/start` e `/control/stop` assenti.
`/health` espone rete, chain ID, digest, altezza, finalità e stato reactor/trasporto.
HTTP 200 significa processo raggiungibile, non prova di quorum o finalità recente.
Il digest identifica i parametri locali; non è un handshake che impedisce a un
peer configurato diversamente di connettersi. Validazione dei messaggi e della
catena continua ad applicarsi. L'ingress deve confrontare rete/configurazione e
finalità prima di instradare traffico pubblico.
RPC con limite corpo e timeout; nessuna nuova promessa EVM, MetaMask o WebSocket.
Prima del deploy pubblico restano gli ID distinti, revisione del recovery,
provisioning condiviso, prove multi-host e ingress TLS autorizzato. La suite netns
RC3 precedente valida il vecchio harness, non sostituisce queste nuove prove.
