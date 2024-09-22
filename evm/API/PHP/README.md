Ecco una versione delle **API in PHP** per il sistema di token e una request personalizzata per gestire i pagamenti automatici agli utenti di **Hashburst** con diverse qualifiche (ad esempio, "dealer" e "reseller") e distribuzione dei proventi delle Pool.

### **PHP API per Hashburst Blockchain**

Prima di tutto, per gestire le API in PHP, useremo **Slim Framework** o **pure PHP** per la gestione delle richieste API. Per semplicità, l'implementazione utilizzerà pure PHP per esporre endpoint come richiesto.

#### **1. Struttura degli Endpoint API**

| Metodo | Endpoint                          | Descrizione                                      |
|--------|------------------------------------|--------------------------------------------------|
| `POST` | `/contracts/mint`                 | Minting di token HBT-20 o HBT-721.               |
| `POST` | `/contracts/transfer`             | Trasferimento di token HBT-20 o HBT-721.         |
| `GET`  | `/contracts/balance`              | Controllo del bilancio di un wallet.             |
| `GET`  | `/contracts/info`                 | Informazioni e transazioni del contratto.        |
| `POST` | `/contracts/auto-withdraw`        | Auto-withdraw da pool verso i wallet di Hashburst.|

### **Implementazione in PHP**

```php
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
```

### **Auto-Withdraw Request**

Questo endpoint gestisce la **distribuzione automatica** dei proventi dalle Pool, garantendo che una parte del minato lordo sia destinata a dealer, reseller, e che il minato netto venga distribuito agli utenti Hashburst in base alle accepted shares:

1. **Dealer e Reseller**: Ricevono una percentuale fissa del minato lordo.
2. **Utenti Hashburst**: Il minato netto viene redistribuito in base alla loro quota accettata nelle Pool.

#### Esempio di richiesta **POST** all'API di auto-withdraw:

```json
{
  "contract_address": "0xYourContractAddress",
  "wallets": ["0xUserWallet1", "0xUserWallet2", "0xUserWallet3"],
  "dealer_wallet": "0xDealerWallet",
  "reseller_wallet": "0xResellerWallet",
  "miner_wallet": "0xMinerWallet",
  "gross_mined_amount": 1000, // Minato complessivo lordo
  "accepted_share": {
    "0xUserWallet1": 0.3,  // 30% delle accepted shares
    "0xUserWallet2": 0.5,  // 50%
    "0xUserWallet3": 0.2   // 20%
  }
}
```

### **Come funziona la Distribuzione Automatica**
- Il **dealer** riceve il 5% del lordo, il **reseller** il 2%.
- Il restante 93% viene distribuito agli utenti proporzionalmente in base alle **accepted shares**.

### **Sicurezza e Autenticazione**
- **Autenticazione**: Utilizzare **OAuth2** o API keys per consentire l'accesso solo a sistemi autorizzati.
- **Crittografia**: Implementare **HTTPS** per proteggere le comunicazioni e impedire attacchi man-in-the-middle.
  
Questo sistema garantisce una distribuzione trasparente e automatizzata dei proventi minati tra i vari stakeholder all'interno dell'ecosistema **Hash

This PHP-based API system facilitates the automatic withdrawal of mined cryptocurrency from pools to the wallets of Hashburst users, while ensuring that dealer and reseller roles receive their appropriate share.

### **Core Functionality Recap**

- **Minting Tokens** (`POST /contracts/mint`): Allows authorized third parties to mint new fungible (HBT-20) or non-fungible (HBT-721) tokens.
- **Transfer Tokens** (`POST /contracts/transfer`): Enables transferring tokens between wallets.
- **Check Token Balance** (`GET /contracts/balance`): Allows external systems to query the balance of a particular wallet.
- **Auto Withdrawal for Pools** (`POST /contracts/auto-withdraw`): Manages the automatic distribution of mined cryptocurrencies across users, with dedicated shares for dealers and resellers.

### **Key Features of Auto Withdrawal Request**

The request for auto-withdrawal splits the mined cryptocurrency among:
1. **Users** (based on their contribution in shares).
2. **Dealers** (who receive a percentage of the total gross mined amount).
3. **Resellers** (who also receive a percentage, though smaller).

### **API Implementation in PHP**

Here’s a breakdown of the implementation for each API:

#### **1. Minting Tokens**
This allows third-party systems to create new tokens in the ecosystem. The request needs to include contract details, recipient wallet, and the amount or metadata (for NFTs).

```php
if ($_GET['action'] === 'mint') {
    // Process mint request here
    $method = 'eth_sendTransaction';
    // Call smart contract mint function
}
```

#### **2. Transfer Tokens**
This API handles token transfers from one user to another. It can work for both fungible (HBT-20) and non-fungible (HBT-721) tokens.

```php
if ($_GET['action'] === 'transfer') {
    // Process token transfer
    $method = 'eth_sendTransaction';
    // Call smart contract transfer function
}
```

#### **3. Check Token Balance**
This endpoint checks the balance of a given user’s wallet, allowing third-party systems to get real-time token holdings.

```php
if ($_GET['action'] === 'balance') {
    // Query balance using eth_call
    $method = 'eth_call';
    // Fetch the balance from smart contract
}
```

#### **4. Auto Withdrawal Request for Pools**

This is the most complex part, where the system automates payments based on user roles and contributions. **Dealer** and **Reseller** wallets receive predefined percentages from the gross mined amount, while the rest is split among users based on their accepted shares.

- **Dealer Share**: 5% of the gross mined amount.
- **Reseller Share**: 2% of the gross mined amount.
- **User Share**: The remaining amount is distributed proportionally based on the user's accepted shares from the pool.

The system sends the relevant amount to each wallet using smart contract transactions:

```php
if ($_GET['action'] === 'auto-withdraw') {
    $gross_mined_amount = $_POST['gross_mined_amount'];
    $net_mined_amount = $gross_mined_amount * 0.93; // Net after dealer and reseller
    
    // Distribute to users based on accepted shares
    foreach ($wallets as $wallet) {
        $amount = $net_mined_amount * $accepted_share[$wallet];
        // Call smart contract to transfer funds
    }
    
    // Transfer to dealer and reseller
    sendRequest('eth_sendTransaction', [
        'from' => $miner_wallet,
        'to' => $dealer_wallet,
        'data' => 'autoWithdraw(' . $dealer_wallet . ', ' . $dealer_amount . ')'
    ]);
    
    sendRequest('eth_sendTransaction', [
        'from' => $miner_wallet,
        'to' => $reseller_wallet,
        'data' => 'autoWithdraw(' . $reseller_wallet . ', ' . $reseller_amount . ')'
    ]);
}
```

#### **Security & Authentication**
To secure the API:
- **API Keys**: Authenticate third-party systems using API keys.
- **HTTPS**: Use HTTPS for encrypted communication.
- **Rate Limiting**: Implement rate limiting to prevent abuse.

### **Next Steps for Integration**
1. Deploy the API using a web server (like **Apache** or **Nginx**) or containerize it using **Docker** for easy scaling.
2. Set up security features such as **OAuth2** and secure the endpoints with **CORS** rules.
3. Test the API using tools like **Postman** or **cURL** to ensure the correct functionality of minting, transfers, balance checks, and auto-withdrawals.

This PHP API setup ensures that third-party systems can interact with the Hashburst blockchain effectively, automating key functions like token minting and withdrawal from mining pools.
