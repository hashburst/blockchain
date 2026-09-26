HVM Testnet Resume v1.0.2
Eseguire dal Mac: shasum -a 256 -c SHA256SUMS && python3 resume.py

Solo per i quattro validatori gia attivati con runtime 06d7c3bd.
Non eseguire activate.py delle versioni precedenti.

Correzione: reactor_status_fresh=false indica TryLock occupato.
Non rappresenta uno stop del reactor. Il gate verifica identita, chain ID,
configurazione, pin, binary hash, journal sano, quattro validatori, tre peer,
radice del validator set condivisa e due campioni con finalita crescente.
I dettagli reactor mancanti non costituiscono prova di assenza di evidence.

Nessuna reinstallazione o promozione: resume permette solo status, restart, recovery.
Il solo nodo v4 viene fermato dopo il gate di avanzamento, controllato offline,
riavviato e verificato per prefisso journal identico e nuovo PRECOMMIT.
Le chiavi non vengono esportate. Nessun reset dati, journal, pin o genesis.
Chain ID testnet 4735490; legacy e mainnet non modificati.
In caso di errore nessun rollback automatico; conservare output e report.

Timeout: autenticazione 180s per nodo; lettura 30s per batch parallelo.
Gate finalita 240s piu al massimo un batch e pausa; controllo riavvio
650s massimo per chiamata (stop/check/start fino a 300s per subprocess).
Se un controllo offline fallisce dopo stop, v4 resta fermo: non cancellare
il marker o ripetere il riavvio alla cieca; inviare il report.
La verifica finale di recupero ha budget 90s piu una chiamata.
Questo pacchetto non modifica il runtime e non corregge i contatori delivery_dropped.
I log originali mostrano finalita oltre 23000: il vecchio timeout era del gate.
