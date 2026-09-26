HVM Network — ingress HTTPS testnet, sola lettura

Destinazione 64.31.4.9, observer gia verificato. Nessun aggiornamento del runtime o riavvio dei validatori.

Dal Mac:
scp HashBurst-HVM-Ingress-v1.0.0.tar.gz root@64.31.4.9:/root/
ssh root@64.31.4.9

Sulla VPS:
cd /root
tar -xzf HashBurst-HVM-Ingress-v1.0.0.tar.gz
cd HashBurst-HVM-Ingress-v1.0.0
sha256sum -c SHA256SUMS && python3 install.py
exit

Dal Mac:
tar -xzf HashBurst-HVM-Ingress-v1.0.0.tar.gz
cd HashBurst-HVM-Ingress-v1.0.0
shasum -a 256 -c SHA256SUMS && python3 verify.py

Endpoint:
https://blockchainapi.one/api/hashburst/hvm/testnet/health
https://blockchainapi.one/api/hashburst/hvm/testnet/rpc
wss://blockchainapi.one/api/hashburst/hvm/testnet/ws

HTTP: allowlist di lettura, singola richiesta JSON-RPC con id; batch/notifiche e metodi di invio esclusi. Corpo massimo 64 KiB, risposta 1 MiB, 16 richieste HTTP upstream contemporanee. Nginx 5 richieste/s per IP, burst 10, 4 connessioni per IP, 64 globali sulle nuove route. Su WebSocket il limite Nginx riguarda l'handshake, non ogni frame: il runtime limita dimensione, numero richieste e durata inattiva della connessione. WS gia esistente in sola lettura; eth_subscribe non implementato.

Il deployment verifica observer, salva il vhost, prova nginx -t, ricarica e attende la route TLS locale. In errore dopo modifica ripristina il vhost; observer e dati restano intatti. Non rieseguire install.py su installazione parziale: inviare errore e BACKUP. La verifica pubblica va eseguita sul Mac.

Questo pacchetto non esegue transazioni, non finanzia account e non implementa EVM. Il canary HVM nativo finanziato e lo sviluppo EVM/MetaMask rimangono passaggi distinti da eseguire. Mainnet non attivata.

Riferimenti Nginx: https://nginx.org/en/docs/http/ngx_http_limit_req_module.html ; https://nginx.org/en/docs/http/ngx_http_limit_conn_module.html ; https://nginx.org/en/docs/http/websocket.html
