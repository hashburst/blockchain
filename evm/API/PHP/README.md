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

### **Sicurezza e Autenticazione**

- **Autenticazione**: utilizzazione di **API Keys** per consentire l'accesso solo a sistemi autorizzati.
- **Crittografia**: implementazione di **HTTPS** per proteggere le comunicazioni e impedire attacchi man-in-the-middle oppure del protocollo **TEP** per la quale questa blockchain è stata ideata nativamente.
  
Questo sistema garantisce una distribuzione trasparente e automatizzata dei proventi minati tra i vari stakeholder all'interno dell'ecosistema **Hashburst**.
