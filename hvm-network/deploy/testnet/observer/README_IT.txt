HVM Network — observer testnet v1.0.0
Destinazione: blockchainapi.one, 64.31.4.9.
Il pacchetto usa il binario API gia distribuito ai validatori (source f7811f75ca1c1a5269d4f4faed0ee0cdcb2bd516) e lo stesso checkpoint testnet approvato ad altezza 7. Non genera genesis. Nessuna chiave di firma dei validatori viene caricata o copiata. La nuova chiave P2P viene creata direttamente sulla VPS.

Dal Mac, nella directory dell'archivio:
scp HashBurst-HVM-Observer-v1.0.0.tar.gz root@64.31.4.9:/root/
ssh root@64.31.4.9

Sulla VPS:
cd /root
tar -xzf HashBurst-HVM-Observer-v1.0.0.tar.gz
cd HashBurst-HVM-Observer-v1.0.0
sha256sum -c SHA256SUMS && python3 install.py
exit

Dal Mac:
tar -xzf HashBurst-HVM-Observer-v1.0.0.tar.gz
cd HashBurst-HVM-Observer-v1.0.0
shasum -a 256 -c SHA256SUMS && python3 verify.py

install.py controlla indirizzo locale, porte libere e connettivita TCP/31307 verso tutti i validatori prima di creare stato. Rifiuta un'installazione observer gia esistente e conserva i file in caso di errore: non rieseguire ciecamente.
verify.py attende fino a due ore la sincronizzazione; stampa avanzamento ogni ciclo. Verifica ruolo, chain ID e digest; journal vuoti; due confronti a un'altezza comune con v1 e avanzamento. I certificati non vengono verificati crittograficamente dal comparatore Python.

Servizio: hashburst-hvm-testnet-ingress.service
Configurazione: /etc/hashburst-hvm-testnet-ingress/node.json
Dati: /var/lib/hashburst-hvm-testnet-ingress
Binario: /opt/hashburst-hvm-testnet-ingress/hashburst-testnet
RPC privato: 127.0.0.1:18009
P2P: TCP/31307

Questo pacchetto non modifica Nginx e non espone RPC pubblici. L'esposizione con filtro metodi e limiti, il WebSocket pubblico e il canary applicativo seguono il confronto live. Non usare /rpc senza filtro come endpoint pubblico: il runtime include metodi legacy da non esporre.
Legacy 1337 invariato; testnet 4735490; mainnet 4735489 non attivata. EVM/MetaMask completo non implementato.
