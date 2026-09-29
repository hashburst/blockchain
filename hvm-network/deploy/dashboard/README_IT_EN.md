# HVM Network dashboard

## Italiano

Eseguire su 64.31.4.9 come root:

    sha256sum -c SHA256SUMS && python3 test-dashboard.py && python3 install-dashboard.py

Installa una scheda nella pagina /hashburst/ esistente; riutilizza il blocco
script con nonce CSP del template PHP. Italiano predefinito, selettore English.
Aggiunge un endpoint GET che inoltra esclusivamente /health dell'observer.
L'interfaccia distingue campioni fermi, avanzamento, errore e dati non disponibili.
Non presenta il numero di peer dell'observer come numero di validatori sani.
Mainnet 4735489 resta indicata non attiva. Non configura o avvia nodi.

L'installer controlla observer, vhost e ancore della pagina; rifiuta una scheda HVM
preesistente di versione diversa. Esegue backup, php -l, nginx -t, reload e prove
HTTPS locali. Su errore ripristina pagina/vhost/snippet. Non reinstalla il runtime.
Il percorso previsto e /var/www/blockchainapi.one/public/hashburst/index.php.
Se differisce, interrompe senza sostituire un'altra pagina.

Dopo il successo verificare dal Mac:

    curl -fsS https://blockchainapi.one/api/hashburst/hvm/testnet/network

Aprire https://blockchainapi.one/hashburst/, selezionare HVM Network, provare
Italiano/English, Aggiorna e controllare che una seconda lettura mostri progresso.
Il controllo pubblico e quello visivo richiedono esecuzione dopo l'installazione.

## English

Run the checksum check, patch tests and installer above on the existing ingress
host. This adds a bilingual HVM tab while retaining the PHP CSP nonce and existing
views. It verifies the local HTTPS page and observer route and restores its own
changes on failure. No node, balance, signing journal or chain configuration is
modified. Mainnet remains explicitly inactive. After installation, verify the
public route and both language buttons in the browser.
