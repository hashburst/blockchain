# HVM Network: recupero del runtime testnet

Questo pacchetto aggiorna il runtime dopo la migrazione APoW già conclusa.
Non eseguire nuovamente `migrate`, non modificare il piano di attivazione e
non eliminare lock, pin, database o journal. I miner restano un comando manuale.
La mainnet non viene attivata da questo pacchetto.

## Prima v3

Copiare il pacchetto sul server v3 e, nella cartella estratta, eseguire:

```
sha256sum -c SHA256SUMS &&
python3 repair-node.py install --node hvm-testnet-v3
python3 repair-node.py verify --node hvm-testnet-v3 --timeout 7200
```

L'installazione richiede root, configurazione testnet/APoW e il vecchio binario
atteso. Conserva una prova locale dei prefissi dei journal e di configurazione/pin,
oltre a una copia degli override. Non legge né esporta chiavi private.
Il nuovo override seleziona esclusivamente il binario verificato e aggiunge il suo
percorso alle eccezioni di esecuzione, senza disattivare le altre protezioni.

Se scade l'attesa, ripetere solo `verify`. Se il servizio fallisce, conservare il
journal systemd: non è previsto rollback automatico. Un'interruzione tra la
registrazione della prova e l'avvio richiede ispezione, non un riavvio alla cieca.

## Gli altri quattro nodi

Solo dopo `RUNTIME_RECOVERY_AND_JOURNAL_PREFIX_OK` su v3, applicare gli stessi
comandi a un nodo per volta: v1, v2, v4 e ingress, sostituendo `--node` con
`hvm-testnet-v1`, `hvm-testnet-v2`, `hvm-testnet-v4`, `hvm-testnet-ingress`.
Ogni verifica deve terminare prima dell'installazione successiva.
L'observer resta senza chiave di consenso e senza firma.

## Accettazione della rete e avvio manuale miner

Dal Mac, nella cartella del pacchetto, usando il piano ORIGINALE della migrazione:

```
python3 coordinator.py verify --plan ../HashBurst-HVM-APoW-Migration-v0.5.0/activation-plan.json --runtime-release runtime-release.json --timeout 7200
```

Questo confronto richiede tutti e cinque i nodi sul nuovo binario, stessa
configurazione e accordo dei commitment a un'altezza finalizzata comune.
Il vecchio piano e i suoi riscontri di migrazione restano immutati.
Dopo il successo, l'operatore può avviare esplicitamente i quattro miner:

```
python3 coordinator.py miners --plan ../HashBurst-HVM-APoW-Migration-v0.5.0/activation-plan.json --runtime-release runtime-release.json
```

Seguono audit di 64 blocchi APoW, reward da 50 HBT per blocco, riavvio singolo e
nuova prova MetaMask. Il comando audit richiede anche il binario auditor e il
relativo audit-release.json della stessa build. Non dichiarare completato il
rollout prima di questi riscontri sulla rete attiva.

## Limiti delle prestazioni e delle release

La cache PoH contiene al massimo 32 risultati calcolati nel processo, identificati
esattamente da input e numero di tick. Non carica risultati da peer o disco e non
modifica la PoH. Accelera le richieste ripetute; non rende gratuita la prima
verifica dell'intera catena. La verifica emette progressi ogni 5000 blocchi.

Questa è una correzione testnet Linux, non una release definitiva mainnet.
Il runtime Windows richiede ancora implementazione e collaudo di locking e
custodia/permessi nativi; un semplice cross-build non ne certifica la sicurezza.
