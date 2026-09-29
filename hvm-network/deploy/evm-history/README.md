# HVM Network: recent historical EVM state

This API-only correction retains detached snapshots of the latest 256 canonical
EVM states, including the head. Balance, nonce, code, storage, eth_call and
eth_estimateGas with an explicit block number use that exact state and block
context. eth_estimateGas without the optional block parameter retains its pending
semantics. No request silently substitutes latest for an unavailable block.

Snapshots are populated after canonical commit and rebuilt for the retained
window during validated chain replay. Rejected or speculative proposals cannot
publish state into the read index. Each response/simulation receives a detached
copy. A slot requires both the canonical block height and hash. This is not an
archive-state implementation: older heights, pre-EVM heights and future heights
return explicit errors. The window does not alter finality or protocol settings.

## Scope and gates

The initial package updates only the existing non-signing testnet observer.
Validators, genesis, chain ID, configuration, runtime pin and signing journals
are not migrated. The observer replays its existing data once during startup.
Its historical snapshots are an in-memory read index, not a second ledger.
Testnet is 4735490; legacy 1337 is unchanged; mainnet 4735489 remains gated.

HTTP historical acceptance is not a MetaMask transaction success claim. After
installation, repeat the browser canary and independently validate the exported
wallet proof. On error retain the proof and any transaction hash; do not repeat
transactions blindly. No automatic funding or signing occurs in this package.

## Installazione IT

Sul Mac, estrarre il pacchetto e verificare SHA256SUMS. Trasferire l'archivio
all'observer 64.31.4.9. Sul server:

    tar -xzf HashBurst-EVM-History-v1.0.0.tar.gz
    cd HashBurst-EVM-History-v1.0.0
    sha256sum -c SHA256SUMS && python3 install-observer.py install

L'installazione richiede il runtime precedente con SHA256
1e52296f36b6666d5fc2d4f7ab1d312d7ded467c73312be946d65211eaac8ba9.
Prima di fermare il servizio verifica identita, ruolo observer, configurazione,
binario e accesso dell'utente di servizio al nuovo eseguibile. Conserva l'unita
precedente e le impronte di configurazione, pin e journal. Non legge chiavi.
Un override modifica solo ExecStart. Non cambia Nginx, gateway o firewall.

Durante il replay, l'ingress puo essere temporaneamente indisponibile; i
validatori continuano a lavorare. L'attesa massima e due ore. Non riavviare il
servizio per abbreviare il replay. Per seguire il progresso, da un altro terminale:

    journalctl -u hashburst-hvm-testnet-ingress.service -n 30 -f

Dopo un'interruzione del terminale o un timeout:

    python3 install-observer.py verify

Questo comando verifica soltanto; non ripete migrazioni o riavvii. In caso di
errore di avvio, conservare l'output e il journal del servizio. Il pacchetto non
esegue rollback automatici. Il precedente eseguibile rimane disponibile.

Sul Mac, dalla directory del pacchetto:

    python3 verify-history.py

Atteso HVM_HISTORICAL_API_FIXED_HEIGHT_OK. Aprire poi:
https://blockchainapi.one/hvm-testnet-metamask/
Selezionare MetaMask, collegare l'account testnet finanziato ed eseguire il
collaudo. Verificare rete HashBurst Testnet prima di ogni conferma wallet.
Scaricare la prova e usare verify-public.py del pacchetto Publication:

    python3 verify-public.py --wallet-proof /percorso/reale/hvm-metamask-public-proof.json

Non dichiarare superato il collaudo senza il marker del wallet e la verifica
on-chain della prova. Un errore residuo richiede metodo e risposta RPC effettivi.

## Installation EN

Verify the package SHA256SUMS, copy it to the observer, extract it and run
`python3 install-observer.py install` as root. The installer updates only the
existing non-signing testnet observer after baseline and identity checks. It
preserves configuration, pin and journals and changes only the executable
through a systemd override. Existing chain replay may take substantial time;
the public API may be unavailable while the four validators keep running.

Use `python3 install-observer.py verify` after a terminal interruption. It never
restarts or migrates state. Run `python3 verify-history.py` from the Mac, then
repeat the browser MetaMask canary on HashBurst Testnet and verify its downloaded
proof with the Publication package. Mainnet activation remains out of scope.

## Reproducible source tests

From hvm-network with Go 1.25.7:

    go test ./blockchain -run 'TestEVMHistor' -count=1
    go test -race ./blockchain -run 'TestEVMHistor|TestEVMRealMempool' -count=1
    go test ./blockchain ./internal/testnet ./internal/evmgateway -count=1

From evm/execution:

    go test ./... -count=1

Installer guards:

    python3 test-package.py
