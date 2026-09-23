# HVM: preparazione rollout testnet prima della mainnet

Stato al 2026-09-23. Questo documento prepara il rollout; non avvia servizi.
Codice revisionato: a033f8ec3c5767b611513b4cd9ac1d1cedf8e3b8.
Baseline legacy preservata: cdcf740fdc0a16eb695c9176142cf848c5e7b9ad.

## Esito revisione e confine di accettazione

L'import in phase3e/ mantiene il modulo Go separato e non cambia alcun file legacy.
Manifest originali e corretti consentono il confronto dei sorgenti.
La suite completa passa dopo l'import. Le evidenze locali comprendono race,
trasporto e loopback. Il transcript VPS registra tutti i gate RC3 a quattro nodi:
packet loss a 49, partition/reconnect a 79, proposer crash a 110.
Le prove sono in review/rc3/. La dicitura journal unavailable nel test lifecycle
è un errore iniettato deliberatamente e il test passa.

Alla verifica della PR #3 non risultano commit status o check-run GitHub:
non presentare questa assenza come CI verde. Nessuna revisione indipendente
o audit di sicurezza completo è attestato da questo documento.
L'esito riguarda la devnet e i gate del runner fornito, non una rete pubblica HVM.

## Riscontri nel codice e lavoro prima dell'attivazione

| Area | Riscontro nel modulo recuperato | Intervento necessario |
|---|---|---|
| Runtime | main.go usa il percorso legacy; il wiring del reactor/trasporto BFT è nel runtime devnet | Realizzare un entry point testnet persistente con configurazione validata, lifecycle e gestione segnali; non installare il fixture runner come servizio pubblico |
| Attivazione | blockchain/protocol_v2.go ha soglie v2 e BFT disabilitate per default | Configurazione condivisa, digest verificabile e altezza concordata prima di firmare |
| Chain ID | 1337 è ancora presente nel runtime/RPC e nei default | Cambiamento separato: ID pubblici distinti, controlli di coerenza tra firma, RPC, consenso e rete; nessuna sostituzione globale delle fixture |
| HVM | hvm/executor.go registra contratti nativi e MiningPayoutRegistry | Definire inizialmente il perimetro testnet HVM nativo |
| Web3 | blockchain/jsonrpc.go espone alcuni eth_* di lettura e metodi hb_*; mancano eth_sendRawTransaction, eth_call, eth_estimateGas e eth_getTransactionReceipt | Implementare e validare il percorso Ethereum prima di dichiarare compatibilità transazionale MetaMask/EVM completa |
| Standard | hvm/standards.go contiene descrittori EVM disponibili/draft | Un catalogo non dimostra esecuzione bytecode o conformità ERC |
| WebSocket | Non risulta un endpoint di sottoscrizione HVM nel modulo recuperato | Implementazione e test eventi/riconnessione separati |
| RPC pubblico | main.go ascolta su 0.0.0.0 e il devnet ha controlli amministrativi | Bind privato, ingress TLS, limiti richieste/compute, allowlist metodi; mai esporre /control/* |
| Rete | Prove netns svolte su un host | Validazione multi-host con identità indipendenti, riavvio processo, catch-up e perdita di quorum |
| Persistenza | Journal e stato fanno parte della sicurezza delle firme | Nessuna condivisione di chiavi tra processi; prova di recupero su dati persistenti e di rifiuto double-sign |

Questi sono requisiti di integrazione testnet/pubblica, non ragioni per perdere
o non versionare il recupero Phase3E già verificato.

## Sequenza operativa

1. Unire la PR di recupero preservando i commit. Bloccare il riferimento del
   sorgente da cui produrre gli artefatti. Conservare hash e log di build.
2. Preparare una release testnet interna con runtime persistente; usare una
   nuova directory di stato, nuove identità testnet e porte distinte, senza
   avviare nulla sulla directory /var/lib/hashburst della rete esistente.
   Non riutilizzare chiavi devnet o i processi temporanei run-*.
3. Definire in un cambiamento separato gli ID HVM Testnet/Mainnet univoci e
   distinti, verificandone i conflitti nei registri pubblici. 1337 resta legacy.
   Stabilire esplicitamente se HVM Mainnet è una nuova rete o una migrazione:
   cambiare chain ID non è una semplice modifica cosmetica della RPC.
4. Definire genesis della sola nuova testnet, rete/bootnodes, validator set,
   poteri/quorum, fee policy e activation heights. Distribuire la stessa
   configurazione verificata a tutti i nodi. Il genesis pubblico esistente
   non viene rigenerato né sovrascritto.
5. Avviare almeno quattro validatori testnet su host distinti ove disponibili,
   uno per identità, con journal persistente. I ruoli HA master/observer della
   DePIN non conferiscono automaticamente il ruolo di validatore blockchain.
6. Verificare hash finalizzati e state root comuni, inclusione transazioni,
   MPR e receipt, riavvio reale, catch-up, isolamento e rientro, perdita di
   quorum senza finalità e ripresa al suo ritorno. Controllare contatori di
   coda/drop e consumo CPU/RAM/disco. Conservare risultati per commit/config.
7. Attivare ingress di staging con controllo di rete/chain ID e stato del
   backend. Failover solo verso nodi della stessa testnet sufficientemente
   sincronizzati. Pubblicare gli endpoint solo dopo scelta degli ID e
   verifica TLS/DNS/limiti. I nomi di dominio HVM non sono assegnati da questo documento.
8. Aggiornare frontend e documentazione con capacità effettivamente provate.
   Per HVM nativa usare hb_*; esporre funzionalità wallet EVM o WS soltanto
   dopo i relativi test. TronLink non è un wallet nativo della L1.
9. Preparare la mainnet con release identificata, piano di migrazione e
   attivazione coordinata, prova su copia coerente dello stato e finestra
   operativa. Richiede approvazione di rollout distinta dal merge.

## Arresto e rollback

Prima dell'attivazione: fermare soltanto i nuovi servizi testnet e ritirare
l'ingress testnet se la configurazione o le verifiche non corrispondono.
Conservare lo stato e i log per diagnosi; nessuna cancellazione automatica.

Dopo firme o finalizzazione: non ripristinare un vecchio journal di firma,
non duplicare una chiave validatore e non riportare indietro il database
indipendentemente dalla rete. Arrestare il nodo interessato e applicare un
recupero che preservi anti-equivocazione e stato finalizzato. Un downgrade
binario richiede compatibilità del formato/protocollo verificata.

## Prossimo deliverable

PR di integrazione runtime testnet persistente e configurazione di rete,
con test di mismatch chain ID/config, persistenza e avvio/arresto.
Il successivo pacchetto di deploy deve fissare commit, hash binari,
validator set e destinazioni prima di avviare servizi.
