# Bootstrap persistente HVM Testnet

Prepara uno stato NUOVO, senza avviare servizi. Runtime revisionato:
06d7c3bd2cff59b2e95b25ed09fe8509dd6de01a. Consenso, rete pubblica e1337 invariati.

## Decisioni richieste prima della generazione

- Chain ID TESTNET approvato, verificato separatamente dai registri pubblici.
  Nessun default. 0,1337 e valori fuori range RPC rifiutati. 987654321 nei test
  e' SOLO un valore isolato di test, non un ID pubblico proposto.
- Quattro host validatori iniziali, IPv4 e TCP31307 raggiungibili fra loro.
  Il runtime blockchain usa libp2p/TCP; TEP separatamente UDP47777.
  Il ruolo HA DePIN non conferisce automaticamente il ruolo validator HVM.
- Chiave pubblica X25519 TEP REALE di ogni host (64 caratteri hex).
  Il builder controlla formato e duplicati, non prova possesso o raggiungibilita'.
- Stessa release runtime su tutti i nodi. Mainnet avra' un altro chain ID.

## 1. Identita' sul proprio host

Impostare TESTNET_CHAIN_ID, NODE_ID, NODE_IPV4, TEP_PUBLIC_KEY_HEX ai valori
approvati. Eseguire sul nodo corretto, directory di destinazione nuova e privata.

```bash
: "${TESTNET_CHAIN_ID:?ID testnet da definire}"
: "${NODE_ID:?ID univoco del nodo}"
: "${NODE_IPV4:?IPv4 del nodo}"
: "${TEP_PUBLIC_KEY_HEX:?Chiave PUBBLICA TEP reale}"
./hashburst-testnet-bootstrap identity \
  --chain-id "$TESTNET_CHAIN_ID" --node-id "$NODE_ID" \
  --ip "$NODE_IPV4" --p2p-port 31307 \
  --tep-public-key "$TEP_PUBLIC_KEY_HEX" \
  --out /root/hvm-testnet-identity
```

Output: public.json e chiavi P2P, consenso, operator e reward nuove. SOLO
public.json va copiato al coordinatore, rinominandolo per nodo. Nessuna chiave
privata lascia il nodo. Operator/reward vanno conservate offline, non caricate
dal runtime. Destinazioni esistenti sono rifiutate, mai rigenerate.

## 2. Checkpoint condiviso: assemblare UNA VOLTA

Raccogliere i quattro file pubblici e verificare i relativi host:

```bash
./hashburst-testnet-bootstrap assemble \
  --chain-id "$TESTNET_CHAIN_ID" --out /root/hvm-testnet-bootstrap \
  /root/hvm-public/node1.json /root/hvm-public/node2.json \
  /root/hvm-public/node3.json /root/hvm-public/node4.json
```

Verifica firme e associazioni operator/node/peer/consensus key, senza ricevere
chiavi private. Crea registrazioni on-chain, finanziamento testnet tramite reward
legacy e validator set. Con4nodi: v2 ad altezza6, checkpoint7, BFT da8.
Parametri TESTNET versionati: bond10coin, activation delay1, unbonding8, jail100,
slash5%; timeout proposal3s, prevote2s, precommit2s, delta0.5s. Non sono parametri
Mainnet approvati. L'ordine dei file definisce l'ordine di bootstrap.

Distribuire lo STESSO bundle completo ai nodi via SCP. Confrontare network.json,
config_digest e SHA256SUMS con il coordinatore su canale fidato. Il bundle non
contiene chiavi private; SHA256SUMS prova integrita', non autenticita' del mittente.
BOOTSTRAP_COMPLETE indica completamento; un output parziale non e' utilizzabile.

Il codice genesis resta quello esistente e puo' produrre lo stesso hash iniziale
legacy. Non dichiariamo un genesis univoco nuovo: chain ID, protocollo, checkpoint
e digest comuni vincolano la testnet. Nessuno stato pubblico viene letto o scritto.
Un genesis con hash specifico per rete sarebbe un requisito protocollo separato.

## 3. Preparazione locale senza avvio

La release06d7c3bd deve essere in staging. APPROVED_RUNTIME_SHA256 proviene dal
pacchetto runtime verificato, APPROVED_NETWORK_DIGEST da network.json approvato.
Nessun percorso operativo deve gia' esistere. EXPECTED_HOST e' il nome noto del
nodo a cui e' destinata la configurazione (es. hpcVM46), non un valore autodetect.

```bash
: "${EXPECTED_HOST:?Hostname atteso}"
: "${APPROVED_NETWORK_DIGEST:?Digest approvato}"
: "${APPROVED_RUNTIME_SHA256:?Hash binario verificato}"
python3 prepare-node.py \
  --host "$EXPECTED_HOST" \
  --bundle /root/hvm-testnet-bootstrap --identity /root/hvm-testnet-identity \
  --node-id "$NODE_ID" --chain-id "$TESTNET_CHAIN_ID" \
  --digest "$APPROVED_NETWORK_DIGEST" --runtime-sha256 "$APPROVED_RUNTIME_SHA256"
```

Crea /etc/hashburst-hvm-testnet/node.json in ruolo OBSERVER e validator.json
separato, chiavi private e /var/lib/hashburst-hvm-testnet con checkpoint, journal
locali e runtime.pin. Esegue provision/check come utente dedicato e verifica anche
la chiave validator contro il registro. Attesi:
HVM_TESTNET_PERSISTENT_STATE_PREPARED / VALIDATOR_KEY_AND_REGISTRY_CHECK_OK /
NO_SERVICE_STARTED. Errori conservano file parziali per diagnosi.

## 4. Attivazione coordinata successiva

Quattro observer NON producono finalita'. Il precedente canary che attende
avanzamento funziona solo dopo l'avvio del quorum validator. Prima verificare
preparazione di tutti i nodi, digest identici, identita', TEP e porte. Poi avviare
i validator in una finestra coordinata preservando journal/recovery snapshot.
Questo pacchetto non avvia nulla e non pubblica RPC (127.0.0.1:18009).
Seguono prove multi-host, hash finalizzati comuni, riavvio, catch-up e quorum.
Non ripristinare journal vecchi, clonare chiavi attive o ricreare il checkpoint
per riparare un nodo. Testnet prima della Mainnet.

## Test riproducibili (Go1.25.7, dal modulo hvm-network)

```bash
GOTOOLCHAIN=local go test ./internal/bootstrap ./cmd/hashburst-testnet-bootstrap -count=1
GOTOOLCHAIN=local go test -race ./internal/bootstrap -count=1
HB_BOOTSTRAP_INTEGRATION=1 GOTOOLCHAIN=local go test ./internal/bootstrap -run TestBootstrapFourProcessesRestart -count=1 -v
```

Solo directory temporanee: runtime reale, quattro processi, chain ID isolata e
riavvio SIGKILL. Non riapre i gate netns/packet-loss HVM Network.
