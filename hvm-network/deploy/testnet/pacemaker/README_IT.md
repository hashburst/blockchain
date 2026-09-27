# HVM Network: recupero del pacemaker testnet

Questa release corregge l'esaurimento dei round sulla testnet 4735490. Non ripete la migrazione EVM. Configurazione, runtime.pin, genesis, database e journal non vengono sostituiti. Il runtime riapre lo stato esistente con tutti i controlli di identità, certificati, lock e firme.

Il limite 64 diventa, per questa testnet, una finestra di messaggi e un limite del backoff. I round firmati continuano monotonamente entro i limiti della rappresentazione int64. I round-change autenticati conservano al massimo una richiesta futura per validatore e richiedono f+1 potere per il catch-up. I vecchi lock certificati sono conservati indipendentemente dalla finestra dei messaggi. Le altre chain ID mantengono il limite precedente. Nessuna modifica a 1337 o attivazione di 4735489.

## Condizioni e manutenzione

Questa procedura richiede il runtime precedente d91ed8eeef0fb9290f74516883d536caf52500878ef89099a81f1a4b6064c8d3 già in esecuzione sui quattro validatori e sull'observer, configurazione migrata con digest 71f6c677b00f42d181a2dc4fa5b1c3dbe50101a2599cac80357aed7fbf8b1021, attivazione 53303, gas 200000, base fee 1. Un altro stato viene rifiutato.

La modalità apply comporta una finestra di manutenzione della sola testnet: prima staging su tutti i nodi, poi arresto coordinato, prove di conservazione, sostituzione del solo ExecStart e avvio. L'accesso SSH root avviene dal Mac e richiede le cinque password. Nessuna chiave privata viene trasferita. Gli indirizzi sono quelli già usati nel rollout.

Sul Mac, dalla cartella estratta:

```sh
shasum -a 256 -c SHA256SUMS && python3 rollout.py apply
```

Il replay può richiedere decine di minuti. La procedura attende al massimo due ore e salva health, diagnostica e commitment per nodo. Quando tutti i nodi rispondono, dieci minuti senza avanzamento della finalità interrompono la verifica con un errore esplicito. Non vengono effettuati rollback di stato o ripetizioni di migrazione.

Dopo una perdita SSH o un timeout, se i servizi sono stati avviati:

```sh
python3 rollout.py verify
```

Non ripetere apply. start-prepared è ammesso soltanto dopo che l'installazione è già completata su tutti e cinque i nodi: controlla tutte le prove prima di avviare un servizio. Un arresto precedente alla fine dell'installazione richiede l'esame dei log; non forza il completamento.

Il gate finale richiede uguaglianza dei commitment alla stessa altezza su cinque nodi, attivazione EVM finalizzata e almeno cinque ulteriori blocchi, stesso binario e prefissi journal conservati. Il confronto usa i certificati verificati dal nodo; non costituisce una verifica crittografica indipendente dei certificati.

Dopo HVM_TESTNET_PACEMAKER_AND_EVM_FINALITY_OK:

```sh
python3 rollout.py restart-v4
```

Questa modalità richiede un gate recente, riavvia soltanto v4 e verifica recupero, prefisso journal, pin e presenza di un nuovo precommit locale. Non cancella il marker di un riavvio già richiesto. In caso di interruzione dopo il marker e prima dell'avvio, controllare i log e lo stato del solo servizio v4: non rimuovere il marker.

## Dashboard blockchainapi.one/hashburst

Dopo il gate di recupero, trasferire lo stesso archivio a root@64.31.4.9, estrarlo e, dalla directory estratta, eseguire:

```sh
sha256sum -c SHA256SUMS && python3 install-dashboard.py
```

L'installer richiede l'observer corretto, modifica esclusivamente il template /var/www/blockchainapi.one/public/hashburst/index.php e aggiunge una location nginx GET per /api/hashburst/hvm/testnet/network. Crea backup, preserva il nonce e la CSP esistenti, esegue php -l e nginx -t, attende il reload e verifica le due risposte locali HTTPS. Se il template differisce dagli anchor verificati, si ferma senza indovinare il percorso. Errori successivi alla modifica ripristinano solo i file della dashboard/nginx.

La nuova scheda mostra l'observer, due campioni di finalità, i peer, il reactor e l'attivazione. Non dichiara automaticamente sana l'intera rete, completati i test MetaMask o attiva la mainnet. L'ingress EVM pubblico, se non ancora installato, viene mostrato come non disponibile. Non viene abilitato dall'installer della dashboard.

## Prove riproducibili e limiti

TEST_RESULT.txt riporta le prove locali e il confronto prima/dopo. Nel repository: Go 1.25.7, CGO_ENABLED=0 go test -count=1 -p 1 ./... da hvm-network e da evm/execution; python3 test-package.py nella cartella deploy. Il build deve partire da un checkout pulito del commit SOURCE_COMMIT.

La release è collaudata localmente; nessun risultato VPS viene presunto. L'esecuzione di apply e il riavvio live restano da verificare. Non sono stati eseguiti nuovi race test. La pubblicazione dell'ingress EVM, le prove effettive MetaMask e la mainnet separata restano gate successivi. Non fare merge come release mainnet sulla sola base di questo pacchetto.
