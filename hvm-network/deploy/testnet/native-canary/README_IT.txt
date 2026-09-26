HVM Network — collaudo nativo finanziato v1.0.1

Eseguire esclusivamente su v1 (77.90.188.153):
  sha256sum -c SHA256SUMS && ./hvm-native-canary

Il programma verifica chain ID 4735490, ruolo validator e identità v1.
Crea /root/hvm-native-canary-v1/account.key (0600) e finanzia questo account
con 1 HBT testnet dall'operator.key già presente su v1. Nessuna chiave esce
dal nodo; nessuna chiave consenso viene letta. Limite commissione per tx:
0.01 HBT. Tre transazioni: trasferimento, deploy MPR, profilo sintetico.
Verifica firme locali, nonce avanzato, fee/compute, receipt, eventi e lettura.
Salva transazioni firmate prima dell'invio; un nuovo avvio riutilizza le
stesse transazioni. Non eliminare la directory del collaudo. Un lock evita
esecuzioni concorrenti. Non riavvia servizi e non modifica dati/journal/pin.

Copiare SOLO public-proof.json sul Mac:
  scp root@77.90.188.153:/root/hvm-native-canary-v1/public-proof.json .
  python3 verify-public.py public-proof.json

Il verificatore confronta receipt e commitment alla stessa altezza fra v1
e l'observer pubblico. Non verifica indipendentemente tutte le firme QC e
non fornisce una prova Merkle di inclusione delle receipt nel commitment.
I test inclusi sono locali: esecuzione nativa, guardie receipt/fee/hash,
persistenza dei byte firmati e rifiuto chain errata. Il risultato LIVE è
ottenuto soltanto dopo i due comandi sopra; non è già certificato.
Questo pacchetto NON implementa EVM, Ethereum subscriptions o MetaMask.
Legacy 1337 e mainnet 4735489 non sono modificati.

Correzione v1.0.1: receipt assente del runtime precedente accettata solo
per hb_getTransactionReceipt; errori RPC riportano metodo e contesto.
Riutilizza /root/hvm-native-canary-v1 senza cancellare o sovrascrivere chiavi.
Il binario validator non viene sostituito. La correzione strutturale JSON-RPC
è inclusa nel repository per la prossima release del runtime.
