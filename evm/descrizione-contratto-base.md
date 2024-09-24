## Descrizione del Contratto##:

### Definizioni ###

Questo smart contract rappresenta la logica di distribuzione dei guadagni minati all'interno dell'ecosistema Hashburst, con integrazione off-chain per gestire la varietà di criptovalute utilizzate per i pagamenti.

**Dealer e Reseller**:

Dealer e Reseller sono due entità che ricevono, secondo le impostazioni di esempio (le quali non sono e non rappresentano le condizioni reali dei contratti in essere), rispettivamente il 5% e il 2% dei guadagni lordi del mining, come definito nelle variabili "dealerPercentage" e "resellerPercentage".
Questi pagamenti vengono trasferiti direttamente ai loro indirizzi "on-chain" tramite la funzione Solidity "transfer".

**Struttura degli Utenti**:

Ogni utente è rappresentato da un indirizzo Ethereum (o Ethereum Classic) per le operazioni on-chain, e una lista di indirizzi per criptovalute diverse (es. BTC, DOGE, LTC, ecc.), che vengono memorizzati nel "mapping users".
Gli indirizzi multi-cripto sono salvati come un "array di stringhe" nella struttura dell'utente (string[] cryptoAddresses), e sono richiamati per inviare pagamenti "off-chain" in diverse criptovalute.

**Distribuzione Automatica dei Fondi**:

Dopo aver detratto le percentuali di dealer e reseller, il contratto calcola le quote nette da distribuire agli utenti in base alle loro accepted shares.
Per ciascun utente, il contratto invia "off-chain", chiamando la funzione "sendCryptoOffChain", il flusso delle richieste di pagamento alle Mining-Pool le quali, grazie all'invio tramite API dei codici API Key, BlockId-Signature, codice identificativo del worker, assegnano le quote approvate in proporzione ai contributi per ogni "coin" estratto (grazie alla miglior performance sul blocco vinto) e creano una coda di prelievi automatizzati ("auto-withdrawal") pronte a diventare, interagendo con le API delle diverse "Chain" per eseguire i trasferimenti nelle criptovalute minate (es. DOGE, LTC, BCH, BTC, ETC; ecc.), transazioni in attea di convalida ("pending") nelle rispettive "mempool".

**Funzioni Off-Chain**:

La funzione "sendCryptoOffChain" contiene le logiche per l'integrazione con API che possono gestire l'invio di fondi in criptovalute come BTC, DOGE, o altre valute minate secondo il principio definito e descritto nel precedente punto.
Questa funzione può essere implementata utilizzando servizi come BlockCypher, CoinPayments, o collegamenti diretti a nodi full per ciascuna blockchain.

**Ricezione di Fondi**:

La funzione receive() estende il contratto per permettere anche di ricevere Ether o altre criptovalute supportate sulla rete "Ethereum/ETC", ad esempio se si volesse raccogliere i guadagni lordi dalle Mining-Pool per diverse forme di distribuzione (che gli sviluppatori e gli utilizzatori potranno definire per i loro progetti).

**Integrazione Off-Chain**:

Pagamento in Criptovalute Specifiche: dal momento che Solidity non supporta nativamente criptovalute come Bitcoin, Dogecoin e altre Altcoin, i pagamenti in queste valute devono essere eseguiti off-chain. Una delle possibili soluzioni è utilizzare un backend esterno che monitori gli eventi emessi dal contratto (FundsDistributed), raccolga i dati necessari e interagisca con le blockchain appropriate (Bitcoin, Dogecoin, ecc.) per inviare i fondi agli indirizzi corretti.
Un caso particolare consiste nell'interagire direttamente con le API delle Mining-Pool (in funzione delle modalità con cui si esegue l'attività estrattiva), mantenendo l'origine da mining ed eliminando intermediazioni del sistema, per permettere i trasferimenti diretti verso i wallet dei partecipanti alla "community" nell'ecosistema. 

**Considerazioni per gli Sviluppatori**:

Sicurezza e Access Control: è importante implementare meccanismi di sicurezza per prevenire abusi. Un suggerimento agli sviluppatori per i propri progetti: si potrebbe limitare l'accesso alla funzione "distributeFunds" solo a un account autorizzato (come un amministratore o un oracolo di mining).

Integrazione delle API: la funzione "sendCryptoOffChain", per tutti coloro che intendono sviluppare i propri progetti, può essere integrata con API di servizi esterni come BlockCypher, Coinbase Commerce o nodi full delle blockchain, per eseguire il pagamento nelle criptovalute corrispondenti.

Audit dei Contratti: poiché questo contratto gestisce fondi reali, è consigliabile sottoporlo a un audit di sicurezza da parte di terze parti specializzate in sicurezza blockchain.
