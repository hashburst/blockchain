Questa è la versione operativa (ovviamente senza riportare le reali configurazioni - coperte da riservatezza - definite dai rapporti contrattuali in essere tra produttori e distributori) delle **API in PHP** per il sistema di token con una "request" personalizzata per gestire i pagamenti automatici agli utenti di **Hashburst** con diverse qualifiche (ad esempio, "dealer" e "reseller") e distribuzione dei proventi delle Pool.

### **PHP API per Hashburst Blockchain**

Prima di tutto, per gestire le API in PHP, per la gestione delle richieste API, l'implementazione sviluppata utilizza useremo **pure PHP**  per esporre endpoint sul perimetro web.

#### **1. Struttura degli Endpoint API**

| Metodo | Endpoint                          | Descrizione                                      |
|--------|------------------------------------|--------------------------------------------------|
| `POST` | `/contracts/mint`                 | Minting di token HBT-20 o HBT-721.               |
| `POST` | `/contracts/transfer`             | Trasferimento di token HBT-20 o HBT-721.         |
| `GET`  | `/contracts/balance`              | Controllo del bilancio di un wallet.             |
| `GET`  | `/contracts/info`                 | Informazioni e transazioni del contratto.        |
| `POST` | `/contracts/auto-withdraw`        | Auto-withdraw da pool verso i wallet di Hashburst.|

### **Implementazione in PHP**

                    php
                    
                    <?php
                      header('Content-Type: application/json');
                      
                      // Dati di connessione a un nodo blockchain (per esempio Ethereum o BSC)
                      $web3_url = 'https://bsc-dataseed.binance.org/'; // Cambiare per la rete specifica
                      $private_key = 'your_private_key';
                      
                      // Funzione per connettersi alla blockchain tramite Web3.php o Guzzle
                      function sendRequest($method, $params) {
                          global $web3_url;
                          $data = [
                              'jsonrpc' => '2.0',
                              'method' => $method,
                              'params' => $params,
                              'id' => 1
                          ];
                      
                          $ch = curl_init($web3_url);
                          curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
                          curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($data));
                          curl_setopt($ch, CURLOPT_HTTPHEADER, ['Content-Type: application/json']);
                          $response = curl_exec($ch);
                          curl_close($ch);
                      
                          return json_decode($response, true);
                      }
                      
                      // Mint Token HBT-20 o HBT-721
                      if ($_SERVER['REQUEST_METHOD'] === 'POST' && $_GET['action'] === 'mint') {
                          $contract_address = $_POST['contract_address'];
                          $to_address = $_POST['to_address'];
                          $amount = $_POST['amount'] ?? null;  // HBT-20
                          $metadata = $_POST['token_metadata'] ?? null;  // HBT-721
                      
                          // Call smart contract mint function
                          if ($amount) {
                              $method = 'eth_sendTransaction';
                              $params = [
                                  'from' => $private_key,
                                  'to' => $contract_address,
                                  'data' => 'mint(' . $to_address . ', ' . $amount . ')'
                              ];
                          } else {
                              $method = 'eth_sendTransaction';
                              $params = [
                                  'from' => $private_key,
                                  'to' => $contract_address,
                                  'data' => 'mintNFT(' . $to_address . ', "' . json_encode($metadata) . '")'
                              ];
                          }
                      
                          $result = sendRequest($method, [$params]);
                          echo json_encode($result);
                          exit;
                      }
                      
                      // Trasferisci Token
                      if ($_SERVER['REQUEST_METHOD'] === 'POST' && $_GET['action'] === 'transfer') {
                          $contract_address = $_POST['contract_address'];
                          $from_address = $_POST['from_address'];
                          $to_address = $_POST['to_address'];
                          $amount = $_POST['amount'] ?? null;
                          $token_id = $_POST['token_id'] ?? null;
                      
                          // Call smart contract transfer function
                          $method = 'eth_sendTransaction';
                          $params = [
                              'from' => $from_address,
                              'to' => $contract_address,
                              'data' => $amount ? 'transfer(' . $to_address . ', ' . $amount . ')' : 'transferFrom(' . $from_address . ', ' . $to_address . ', ' . $token_id . ')'
                          ];
                      
                          $result = sendRequest($method, [$params]);
                          echo json_encode($result);
                          exit;
                      }
                      
                      // Controlla il Bilancio
                      if ($_SERVER['REQUEST_METHOD'] === 'GET' && $_GET['action'] === 'balance') {
                          $contract_address = $_GET['contract_address'];
                          $wallet_address = $_GET['wallet_address'];
                      
                          // Call smart contract balanceOf function
                          $method = 'eth_call';
                          $params = [
                              'to' => $contract_address,
                              'data' => 'balanceOf(' . $wallet_address . ')'
                          ];
                      
                          $result = sendRequest($method, [$params]);
                          echo json_encode($result);
                          exit;
                      }
                      
                      // Auto-withdraw per le Pool
                      if ($_SERVER['REQUEST_METHOD'] === 'POST' && $_GET['action'] === 'auto-withdraw') {
                          $contract_address = $_POST['contract_address'];
                          $wallets = $_POST['wallets']; // Lista dei wallet degli utenti
                          $dealer_wallet = $_POST['dealer_wallet']; // Wallet del dealer
                          $reseller_wallet = $_POST['reseller_wallet']; // Wallet del reseller
                          $miner_wallet = $_POST['miner_wallet']; // Wallet del miner
                          $gross_mined_amount = $_POST['gross_mined_amount']; // Provento lordo
                      
                          // Calcolo delle percentuali per dealer e reseller
                          $dealer_percentage = 0.05; // 5% per dealer
                          $reseller_percentage = 0.02; // 2% per reseller
                          $net_mined_amount = $gross_mined_amount * (1 - $dealer_percentage - $reseller_percentage);
                      
                          // Distribuzione automatica
                          foreach ($wallets as $wallet) {
                              $accepted_share = $_POST['accepted_share'][$wallet]; // Accepted share per il wallet
                              $amount = $net_mined_amount * $accepted_share; // Proporzionale alle shares
                      
                              $params = [
                                  'from' => $miner_wallet,
                                  'to' => $wallet,
                                  'data' => 'autoWithdraw(' . $wallet . ', ' . $amount . ')'
                              ];
                      
                              sendRequest('eth_sendTransaction', [$params]);
                          }
                      
                          // Pagamento al dealer
                          $dealer_amount = $gross_mined_amount * $dealer_percentage;
                          sendRequest('eth_sendTransaction', [
                              [
                                  'from' => $miner_wallet,
                                  'to' => $dealer_wallet,
                                  'data' => 'autoWithdraw(' . $dealer_wallet . ', ' . $dealer_amount . ')'
                              ]
                          ]);
                      
                          // Pagamento al reseller
                          $reseller_amount = $gross_mined_amount * $reseller_percentage;
                          sendRequest('eth_sendTransaction', [
                              [
                                  'from' => $miner_wallet,
                                  'to' => $reseller_wallet,
                                  'data' => 'autoWithdraw(' . $reseller_wallet . ', ' . $reseller_amount . ')'
                              ]
                          ]);
                      
                          echo json_encode(['status' => 'success']);
                          exit;
                      }

### **Opzioni di Nomenclatura e Percorso API RESTful**

1. iFront Controller: se scegli di utilizzare index.php, puoi strutturare gli endpoint in questo modo:

                    Percorso API: /contracts/mint
   
                    File PHP: /contracts/index.php
   
In questo caso, il web server Apache può essere configurato tramite il file ".htaccess" o direttamente nella configurazione del virtual host specifico del sito per nascondere il file "index.php", facendo apparire l'URL come "/contracts/mint", ecc. Il file ".htaccess" o la configurazione di Apache reindirizza tutte le richieste che non puntano a un file o una directory esistenti verso index.php.

Esempio .htaccess per nascondere index.php:

                    apache
                    
                    RewriteEngine On
                    RewriteCond %{REQUEST_FILENAME} !-f
                    RewriteCond %{REQUEST_FILENAME} !-d
                    RewriteRule ^ index.php [QSA,L]

Con questa configurazione, la struttura degli URL sarà:

- POST /contracts/mint
- POST /contracts/transfer
- GET /contracts/balance
- GET /contracts/info
- POST /contracts/auto-withdraw

Il file index.php gestirà tutte queste richieste e le instraderà in base al metodo HTTP e al percorso richiesto.
Per creare un'API RESTful con URL puliti, tipo "/contracts/mint", "/contracts/transfer", e così via, è possibile utilizzare .htaccess per rendere più leggibili gli URL e nascondere il nome del file front controller (index.php) oppure modificare la configurazione specifica del vierual host nel web server Apache per maggir controllo sulla gestione del routing ed efficienza prestazionale in quanto letta una sola volta e non ad ogni richiesta.

####  Integrare i Wallet Multi-Crypto con lo Smart Contract

Nel contesto di Hashburst, ogni utente ha uno o più wallet associati a diverse criptovalute che sono archiviate nei file corrispondenti al loro "BlockId-Signature" (ad esempio "f**2*e20"). Questo file contiene gli indirizzi di ogni criptovaluta minata dall'utente, ad esempio:

                    DOGE: DHxy9PV7f7xo8eUD3j*************oLy
                    ETC: 0x82f138f37506092927b******************aaf
                    BTC: 158kWeRYQjiPX759E**************pHX
                    XMR: 429sqZ7wwuderVmqKbyZHi9LNWjCm9kpVNSUVsVFcJaFdd*********************************************nCrF

#### Funzionamento:

**1 BlockId-Signature**: ogni utente ha un file univoco nella directory "/blockchain/ledger/wallets" che memorizza gli indirizzi per ogni criptovaluta minata. Questi indirizzi vengono utilizzati per inviare i pagamenti automatici dalle pool.

**2 Distribuzione Automatica Basata sulle Accepted Shares**: il contratto intelligente o il sistema di gestione esterno recupera gli indirizzi specifici dal file del "BlockId-Signature" dell'utente e genera il flusso ("Stream") per i payout. Ad esempio:

- Se l'utente ha minato Dogecoin, il pagamento verrà instradato all'indirizzo DOGE presente nel file (DHxy9PV7f7xo8eUD3j*************oLy).
- Se ha minato Bitcoin, il sistema utilizzerà l'indirizzo BTC (158kWeRYQjiPX759E**************pHX).

**3 Smart Contract Multi-Wallet**: lo smart contract deve essere progettato per interagire con più blockchain (Ethereum per ERC-20/721 e gli altri protocolli per criptovalute come DOGE, BTC, BCH, LTC, DASH, ETC e così via). Lo smart contract può gestire il flusso ("Stream") dei pagamenti utilizzando API di pagamento multi-blockchain o tramite integrazioni con nodi specifici per ciascuna criptovaluta.

### **Auto-Withdraw Request**

Questo endpoint gestisce la **distribuzione automatica** dei proventi dalle Pool, garantendo che una parte del minato lordo sia destinata a dealer, reseller, e che il minato netto venga distribuito agli utenti Hashburst in base alle accepted shares (ovvero proporzionalmente ai contributi di ogni worker/sub-account):

1. **Dealer e Reseller**: ricevono una percentuale fissa del minato lordo.
2. **Utenti Hashburst**: il minato netto viene redistribuito in modo direttamente proporzionale alla loro quota accettata nelle Pool.

#### Esempio di richiesta **POST** all'API di auto-withdraw:

                    json
                    
                    {
                      "contract_address": "0xYourContractAddress", // Formato degli indirizzi Ethereum (standard ERC-20/721)
                      "wallets": ["UserWallet1", "UserWallet2", "UserWallet3"], // Altri protocolli per le criptovalute estratte DOGE, BTC, BCH, LTC, DASH, ETC, ecc.
                      "dealer_wallet": "DealerWallet", // Altri protocolli per le criptovalute estratte DOGE, BTC, BCH, LTC, DASH, ETC, ecc.
                      "reseller_wallet": "ResellerWallet", // Altri protocolli per le criptovalute estratte DOGE, BTC, BCH, LTC, DASH, ETC, ecc.
                      "miner_wallet": "MinerWallet", // Altri protocolli per le criptovalute estratte DOGE, BTC, BCH, LTC, DASH, ETC, ecc.
                      "gross_mined_amount": 1000, // Minato complessivo lordo
                      "accepted_share": {
                        "0xUserWallet1": 0.3,  // 30% delle accepted shares
                        "0xUserWallet2": 0.5,  // 50%
                        "0xUserWallet3": 0.2   // 20%
                      }
                    }


### **Funzionamento della Distribuzione Automatica**

- Il **dealer** riceve, ad esempio, il 5% del lordo, il **reseller** il 2% (queste quote sono puramente d'esempio e non corrispondono a quelle in essere).
- Il restante 93% viene distribuito agli utenti proporzionalmente in base alle **accepted shares**.

  #### Tipologia di Wallet e Standard
  
  I placeholder dei wallet sono esempi di indirizzi blockchain utilizzati per distribuire ricompense derivanti dall’attività di mining o tokenizzazione all'interno dell'ecosistema Hashburst:

  - User Wallets: la lista ["UserWallet1", "UserWallet2", "UserWallet3"] rappresenta i wallet personali degli utenti che partecipano al mining. Gli utenti ricevono pagamenti basati sulle loro accepted shares delle criptovalute minate nelle pool (ad esempio, DOGE, BTC, ETC, XMR).
  - Dealer Wallet: "DealerWallet" è il portafoglio del "dealer" il quale è un'entità finanziatrice o affiliata che riceve una percentuale fissa dei guadagni lordi minati dalle Pool.
  - Reseller Wallet: "ResellerWallet" è il portafoglio del reseller il quale è un partner che ha diritto a una percentuale sui guadagni lordi derivanti dal mining, tipicamente inferiore a quella del dealer.

  Attenzione: questi wallet non ricevono token che rappresentano il contratto, ma pagamenti diretti dalle Pool in criptovalute (come BTC, DOGE, ETC, XMR, ecc.) in base alle loro accepted shares o diritti contrattuali (per dealer e reseller).

  **Riepilogo: come vengono remunerati questi Wallet?**
  
  Gli utenti ricevono pagamenti nelle criptovalute minate (ad esempio DOGE, BTC, ecc.) proporzionalmente alle accepted shares accumulate. Il dealer e il reseller ricevono una percentuale del guadagno lordo (prima che venga distribuito agli utenti), tipicamente pagato nella stessa criptovaluta che è stata minata (o eventualmente in una specifica valuta concordata). In pratica, il contratto intelligente (smart contract) funziona come intermediario tra la Pool di Mining e i Wallet degli utenti, gestendo la distribuzione automatica delle ricompense.

  **Corrispondenza tra Hashburst Wallet, BlockId-Signature e lista dei Wallet delle Criptovalute specifiche estratte**
  
  Ogni wallet in Hashburst è associato a una lista di indirizzi specifici per ogni criptovaluta "minata" (ad esempio BTC, DOGE, ETC, XMR, ecc.). Queste informazioni sono archiviate fisicamente in file associati agli utenti, denominati con il loro "BlockId-Signature" (ad esempio, "f**2*e20").
  
  Ecco un esempio di come ogni utente può rintracciare in qualsiasi nodo distribuito della blockchain il file nel percorso "/blockchain/ledger/wallets" che contiene i suoi indirizzi per diverse criptovalute:
                  
                      Path: /blockchain/ledger/wallets/f**2*e20
  
                      Plaintext
  
                      DOGE: DHxy9PV7f7xo8eUD3j*************oLy
                      ETC: 0x82f138f37506092927b******************aaf
                      BTC: 158kWeRYQjiPX759E**************pHX
                      XMR: 429sqZ7wwuderVmqKbyZHi9LNWjCm9kpVNSUVsVFcJaFdd*********************************************nCrF
  
    **Processo di abbinamento**
    
  - **Identificazione dell'utente**: il sistema identifica l'utente in base al suo "BlockId-Signature", che rappresenta una chiave unica nel file system della blockchain. L'interoperabilità tra le varie chain, data dall'interfacciamento via API, consente di inviare un flusso ("Stream") di questi dati verso le Pool le quali, grazie agli API Key, ai codici identificativi degli apparati e alle Signature, riconoscono l'utente come sub-account del cluster-miner principale e per ognuno avvia automaticamente la registrazione di procedure di "auto-withdrawal" con gli importi assegnati (già convalidati per il mining-wallet che rappresenta l'account generale di cui i sub-account/worker fanno parte) avviando una coda interna ("queue") dei payout. Questa parte precede la fase di inserimento della transazione nella mempool dei nodi in attesa di entrare in un nuovo blocco e la transazione definitiva in blockchain.
    N.B.: partendo dal mining-wallet della Pool di Mining e andando direttamente al wallet del sub-account, ogni transazione è inclusa in un blocco che contiene la provenienza, l'origine da mining, ovvero la transazione coinbase che include la ricompensa del blocco.
  
  - **Associazione dei wallet**: ogni utente ha un file che contiene i suoi indirizzi wallet per ogni criptovaluta. Quando la Pool genera un pagamento, il sistema abbina i guadagni a uno specifico indirizzo di criptovaluta (ad esempio, invia DOGE al wallet DOGE dell'utente, BTC al wallet BTC, e così via).
  
  - **Smart Contract e pagamenti**: lo smart contract utilizza queste informazioni per consentire alla Mining-Pool di eseguire direttamente le transazioni verso gli indirizzi specifici, assicurando che ogni criptovaluta sia trasferita al wallet giusto conservando l'origine ovverosia la provenienza da una transazione coinbase.
  
  **Passaggio delle Informazioni tramite il Token e lo Smart Contract**:
  
  Le informazioni sui wallet specifici e gli indirizzi delle criptovalute sono fondamentali per l'esecuzione automatica dei pagamenti. Ecco come queste informazioni possono essere gestite tramite Smart Contract:
  
  - **Memorizzazione dei Wallet nel Contratto**: quando un utente si iscrive o viene aggiunto al sistema, i suoi indirizzi wallet per diverse criptovalute vengono associati al suo "BlockId-Signature" e memorizzati nel contratto intelligente o in un sistema di gestione dei dati fuori catena (ad esempio, IPFS o un database decentralizzato).
  
  - **Tokenizzazione delle Ricompense**: ogni share di mining può essere rappresentata da un token virtuale o direttamente dal credito in criptovaluta in funzione delle condizioni d'ambiente e degli scenari in cui l'attività estrattiva dei cluster si svolge. Il **principio fondamentale** è che gli utenti **non** ricevono un token che rappresenta il contratto, ma il contratto gestisce il flusso ("Stream") informativo che consente alle Mining-Pool di mettere in coda di convalida ed esecuzione le transazioni dei fondi minati direttamente destinati ai loro indirizzi wallet in qualità di sub-account (ovvero di worker del cluster-miner) riconosciuti e anonimi (protetti dalla blockchain Hashburst).
  
  - **Esecuzione del Pagamento**: il contratto intelligente riceve i dettagli dei wallet dal flusso ("Stream") associato al "BlockId-Signature" dell'utente e instrada il pagamento ("payout") alla criptovaluta corretta (ovvero controllo di coerenza tra "coin", "address"/"wallet", "mainnet"/2network" e "chain") inviando all'API delle Mining-Pool i dati aggiornati per generare le code di "auto-withdraw" necessari alla distribuzione delle "reward".
  
  **Corollario**
  
  **User Wallets**: sono indirizzi che ricevono pagamenti in criptovalute basate sulle Pool in cui gli utenti stanno minando. Ogni wallet è specifico per una criptovaluta (ad esempio: DOGE, BTC, BCH, ETC, ecc.).
  
  **Dealer e Reseller Wallets**: sono destinatari di una parte fissa dei guadagni lordi derivanti dal mining, pagati in una criptovaluta specifica (come BTC).
  
  **Smart Contract**: gestisce la distribuzione automatica dei fondi, prelevando le informazioni contenute nei "BlockId-Signature" degli utenti per consentire alla Pool di indirizzare correttamente i pagamenti ai rispettivi wallet in base alla criptovaluta minata.
  
  In questo modo, l'infrastruttura è in grado di eseguire distribuzioni automatiche delle ricompense, assicurando che ogni wallet riceva il pagamento nella criptovaluta corretta associata alle Pool di Mining.

### **Sicurezza e Autenticazione**

- **Autenticazione**: utilizzazione di **API Keys** per consentire l'accesso solo a sistemi autorizzati.
- **Crittografia**: implementazione di **HTTPS** per proteggere le comunicazioni e impedire attacchi man-in-the-middle oppure del protocollo **TEP** per la quale questa blockchain è stata ideata nativamente.
  
Questo sistema garantisce una distribuzione trasparente e automatizzata dei proventi minati tra i vari stakeholder all'interno dell'ecosistema **Hashburst**.
