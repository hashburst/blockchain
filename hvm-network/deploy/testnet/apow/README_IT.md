# HVM Network APoW: preparazione del rollout testnet

Questo pacchetto contiene il runtime Linux amd64 con migrazione offline APoW,
il miner di riferimento e il controllo in sola lettura dei quattro validatori e
dell'observer. Non avvia automaticamente la nuova regola di consenso.

## Eseguire ora sul Mac

```
shasum -a 256 -c SHA256SUMS &&
python3 preflight.py
```

Vengono richieste le password SSH dei cinque nodi. Il programma legge health,
configurazione pubblica, pin e metadati del servizio; calcola il digest del binario
in esecuzione, confronta i commitment a un'altezza finalizzata comune e controlla
l'avanzamento. Non legge chiavi private e non arresta, riavvia o riconfigura servizi.
I certificati sono confrontati tramite i nodi: non esegue una nuova verifica
crittografica indipendente delle firme QC.

Il report distingue i processi che usano gia' questo binario. Un errore conserva
le evidenze e termina; non viene ripetuta alcuna mutazione. Alla fine stampa
REPORT_ARCHIVE: restituire quel piccolo archivio per fissare il piano sulla rete
reale. Il runtime attuale puo' essere precedente a questa release: e' previsto.

## Migrazione implementata, da usare con il piano concordato

Il nuovo comando offline e':

```
hashburst-testnet --config /etc/hashburst-hvm-testnet/node.json --migrate-apow /percorso/candidate.json
```

Per l'observer cambia il percorso in /etc/hashburst-hvm-testnet-ingress/node.json.
Non eseguire questo comando durante il preflight. Richiede esclusione del processo
sulla directory dati, candidato identico al precedente salvo protocol.apow e
attivazione futura con margine superiore a 1000 blocchi. Il margine minimo non e'
una garanzia temporale per migrazioni lunghe: il piano deve coordinare tutti i nodi.
L'intento e' persistente, e una ripetizione dello stesso candidato riprende la
transizione tra pin e configurazione. Non cancella database, lock BFT o journal.

Il gas originale protocol.evm.gas_limit NON va cambiato. L'eventuale nuovo limite
si trova in protocol.apow.gas_limit e vale solo dall'attivazione APoW; lo storico
mantiene il limite originale. Due milioni di gas sono un valore sperimentale
collaudato per i contratti strict, non una scelta mainnet automaticamente approvata.

Prima di pianificare l'attivazione servono: parametri di difficolta' calibrati,
miner operativi con chiavi separate e beneficiari identificati, migrazione
coordinata, collaudo dei quattro nodi con observer, perdita di pacchetti e riavvio.
Dopo l'attivazione ogni blocco richiede lavoro APoW valido; l'assenza di miner
impedisce l'avanzamento. Il miner incluso esegue un solo job con timeout: non e'
un servizio continuo pronto a sostituire il provisioning dei miner.

Le reti legacy 1337 e testnet 4735490 rimangono distinte. Questo pacchetto non
attiva mainnet 4735489, non importa fondi legacy e non assegna il miliardo iniziale.
La mainnet richiede il proprio manifesto economico e identita' dedicate.
