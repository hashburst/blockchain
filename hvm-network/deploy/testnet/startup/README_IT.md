# HVM Network: recovery prima del P2P

Il 27 settembre v1 ha completato replay e controllo locale a 51473, poi ha ricevuto blocchi durante l'avvio P2P ed e' terminato con recovery height/parent mismatch. Il codice leggeva il recovery mentre il sync poteva modificare la head. La correzione avvia sincronicamente il reactor, quando il consenso e' gia' abilitato, prima di costruire il P2P. Run riutilizza Start idempotente e gestisce i timer. I controlli di checksum, identita', configurazione, parent, QC, lock e journal sono invariati. Nessun recupero forzato di snapshot incoerenti. Gli observer pre-consenso conservano il percorso di sync verso l'attivazione.

## Esecuzione dal Mac

Estrarre questa release in una cartella nuova, poi:

```sh
shasum -a 256 -c SHA256SUMS && python3 rollout.py apply
```

Questo apply e' specifico di Startup v1.0.1: non eseguire apply dal precedente pacchetto Pacemaker. Richiede il runtime Pacemaker SHA256 1fcdb11df7a23d06a22a7aa56cb3fc5034088129cad6f098a6c2bf32a54e988f e le sue prove preservation.json su ogni nodo. Richiede la configurazione EVM gia' migrata, altezza 53303, gas 200000, base fee 1. Legacy 1337 invariata; nessuna mainnet 4735489 viene attivata.

L'aggiornamento procede v1, v2, v3, v4, observer. Prima di fermare ogni nodo verifica accordo e avanzamento degli altri validatori. v1 puo' essere ancora in replay: la sua API non e' necessaria per lo staging, ma processo, binario, override, identita', configurazione, pin e prefissi journal devono corrispondere alle prove conservate. Dopo ciascun arresto registra gli hash dei file, cambia soltanto ExecStart e attende recupero e catch-up prima del nodo successivo. Nessun database, journal o recovery snapshot viene riscritto dall'installer. Il normale runtime puo' estendere lo stato e i journal.

Il replay resta completo e puo' richiedere circa 40 minuti per nodo come osservato su v1; il tempo non e' una prova di errore. Il limite e' due ore per avvio. Un nuovo crash/riavvio automatico interrompe l'attesa; non viene nascosto con altri tentativi. I log vengono salvati nella cartella startup-results-*.

Se SSH si interrompe dopo l'avvio, apply riconosce un processo che esegue gia' il nuovo binario e lo attende senza riavviarlo. Uno stato parziale tra stop e start viene invece rifiutato e richiede esame dei log: non cancellare preservation.json e non eseguire migrazioni. Per la sola verifica, quando tutti i nodi sono stati aggiornati:

```sh
python3 rollout.py verify
```

Il gate HVM_TESTNET_STARTUP_AND_EVM_FINALITY_OK richiede i cinque nodi, commitment uguali alla stessa altezza, finalita' oltre 53303 piu' cinque blocchi, stesso nuovo binario e conservazione delle prove. I certificati sono verificati dai nodi; il comparatore non e' un verificatore crittografico indipendente.

Dopo quel gate:

```sh
python3 rollout.py restart-v4
```

Questa azione verifica il riavvio persistente di v4, il prefisso journal e un nuovo precommit locale. La prova live resta da eseguire. Ingress pubblico EVM, transazioni applicative, receipt/log, sottoscrizioni e conferme reali MetaMask restano i successivi gate. Questa release non certifica mainnet o piena compatibilita' Ethereum.

## Riproducibilita'

Da checkout pulito di SOURCE_COMMIT, con Go 1.25.7:

```sh
cd hvm-network
CGO_ENABLED=0 GOTOOLCHAIN=local go test -p 1 -count=1 ./...
CGO_ENABLED=0 GOTOOLCHAIN=local HB_TESTNET_INTEGRATION=1 go test -p 1 -count=1 -timeout 12m ./internal/testnet
python3 deploy/testnet/startup/test-package.py
python3 deploy/testnet/startup/build-package.py --out /tmp/hvm-startup-release
```

I test creano solo fixture temporanee. Non rigenerano il genesis della rete pubblica.

## Ripresa v1.0.1

Il binario e' identico a v1.0.0 (SHA256 1e52296f36b6666d5fc2d4f7ab1d312d7ded467c73312be946d65211eaac8ba9). Solo il coordinatore cambia: i timeout e connection-refused di letture health/commitment durante il catch-up vengono ritentati entro il limite di due ore, mantenendo gli errori di identita', divergenza, certificato o SSH come terminali. L'attesa finale senza progresso verificato resta limitata a dieci minuti.

Dai log startup-results-qh5tbmu4: v1 aggiornato, attivo, 4 peer, altezza 52011 e release/prefix verificati; v2/v3/v4/observer a 53542. Il timeout successivo ha interrotto il coordinatore prima dell'aggiornamento di v2. La ripresa apply riconosce v1 e non lo riavvia; attende il catch-up di tutti e cinque anche nel percorso ALREADY_UPDATED_NO_RESTART prima di procedere. Una singola risposta health positiva non basta piu' a saltare questa barriera.

Estrarre v1.0.1 in una nuova cartella sul Mac, controllare SHA256SUMS ed eseguire python3 rollout.py apply. Non occorre copiare plan.json o ripetere la migrazione. I risultati VPS e l'accordo finale restano da acquisire.
