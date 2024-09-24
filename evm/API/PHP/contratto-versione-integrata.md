Per implementare una chiamata **off-chain** per eseguire pagamenti in criptovalute diverse (come **BTC**, **DOGE**, **DASH**, **BCH**, **LTC**, **ETC**, **XMR**), possiamo usare API esterne come **BlockCypher**, **Coinbase Commerce** o direttamente integrare **nodi full** per ciascuna blockchain. 
Per questa versione pubblica in questo rapository, si opta per una logica basata sull'integrazione di **BlockCypher API** per eseguire pagamenti in criptovalute diverse.

### **Integrazione con BlockCypher API**

**BlockCypher** supporta diverse criptovalute come **BTC**, **LTC**, **DOGE**, **DASH** e offre API che consentono di inviare transazioni direttamente sulle loro blockchain.

#### **1. Setup: Configurazione e API Key**

Prima di tutto, è necessario ottenere un'API key da **BlockCypher**. La chiamata API richiede un header con l'API key e i dettagli della transazione.

### **Funzione `sendCryptoOffChain`**

Ecco come effettuare una chiamata **HTTP POST** a BlockCypher per inviare transazioni in criptovalute specifiche.  Come backend off-chain si utilizza la libreria **Guzzle** in PHP la quale eseguirà la chiamata API:

                    solidity
                    
                    // SPDX-License-Identifier: MIT
                    pragma solidity ^0.8.0;
                    
                    contract HashburstPoolDistributor {
                        address public dealer;
                        address public reseller;
                        address public admin;
                    
                        uint256 public dealerPercentage = 5;
                        uint256 public resellerPercentage = 2;
                        uint256 public totalShares;
                    
                        struct User {
                            address userAddress;
                            uint256 acceptedShares;
                            string[] cryptoAddresses;
                        }
                    
                        mapping(address => User) public users;
                        address[] public userAddresses;
                    
                        // Eventi
                        event FundsDistributed(address user, uint256 amount, string cryptoAddress);
                        event DealerPaid(address dealer, uint256 amount);
                        event ResellerPaid(address reseller, uint256 amount);
                    
                        // Solo admin
                        modifier onlyAdmin() {
                            require(msg.sender == admin, "Solo l'amministratore puo' chiamare questa funzione");
                            _;
                        }
                    
                        constructor(address _dealer, address _reseller, address _admin) {
                            dealer = _dealer;
                            reseller = _reseller;
                            admin = _admin;
                        }
                    
                        function addUser(address _userAddress, uint256 _acceptedShares, string[] memory _cryptoAddresses) public onlyAdmin {
                            require(_cryptoAddresses.length > 0, "L'utente deve avere almeno un indirizzo crypto.");
                            users[_userAddress] = User({
                                userAddress: _userAddress,
                                acceptedShares: _acceptedShares,
                                cryptoAddresses: _cryptoAddresses
                            });
                            userAddresses.push(_userAddress);
                            totalShares += _acceptedShares;
                        }
                    
                        function distributeFunds(uint256 grossMinedAmount) public onlyAdmin {
                            require(totalShares > 0, "Non ci sono utenti con shares.");
                    
                            uint256 dealerShare = (grossMinedAmount * dealerPercentage) / 100;
                            uint256 resellerShare = (grossMinedAmount * resellerPercentage) / 100;
                            uint256 netMinedAmount = grossMinedAmount - dealerShare - resellerShare;
                    
                            payable(dealer).transfer(dealerShare);
                            emit DealerPaid(dealer, dealerShare);
                    
                            payable(reseller).transfer(resellerShare);
                            emit ResellerPaid(reseller, resellerShare);
                    
                            for (uint256 i = 0; i < userAddresses.length; i++) {
                                address userAddress = userAddresses[i];
                                User memory user = users[userAddress];
                                uint256 userShare = (netMinedAmount * user.acceptedShares) / totalShares;
                    
                                for (uint256 j = 0; j < user.cryptoAddresses.length; j++) {
                                    string memory cryptoAddress = user.cryptoAddresses[j];
                                    sendCryptoOffChain(cryptoAddress, userShare);
                                    emit FundsDistributed(userAddress, userShare, cryptoAddress);
                                }
                            }
                        }
                    
                        // Funzione per inviare pagamenti off-chain tramite API esterne come BlockCypher
                        function sendCryptoOffChain(string memory _cryptoAddress, uint256 _amount) private pure {
                            // Implementazione off-chain tramite API esterna, chiamata su backend
                            // Per esempio: BlockCypher per BTC, LTC, DOGE, DASH
                            // Il backend (PHP o Node.js) gestisce la chiamata API vera e propria
                        }
                    
                        receive() external payable {}
                    }

### **Backend PHP con BlockCypher API**

Script **PHP** base (ogni sviluppatore eseguirà le propria implementazione) per il backend che esegue la chiamata API a **BlockCypher** per inviare pagamenti in diverse criptovalute.
Le richieste HTTP sono gestite dalla libreria **Guzzle**.

#### **PHP (Guzzle) Script per Pagamenti Off-Chain**

                    php
                    
                    <?php
                    require 'vendor/autoload.php';  // Guzzle
                    
                    use GuzzleHttp\Client;
                    
                    function sendCryptoPayment($cryptoType, $toAddress, $amount, $apiKey) {
                        $client = new Client();
                        $apiUrl = "https://api.blockcypher.com/v1/";
                    
                        // Determina il tipo di criptovaluta (BTC, LTC, DOGE, ecc.)
                        switch($cryptoType) {
                            case 'BTC':
                                $apiUrl .= "btc/main/txs/new";
                                break;
                            case 'LTC':
                                $apiUrl .= "ltc/main/txs/new";
                                break;
                            case 'DOGE':
                                $apiUrl .= "doge/main/txs/new";
                                break;
                            case 'DASH':
                                $apiUrl .= "dash/main/txs/new";
                                break;
                            default:
                                throw new Exception("Unsupported cryptocurrency");
                        }
                    
                        // Crea la transazione
                        $response = $client->post($apiUrl, [
                            'json' => [
                                'inputs' => [['addresses' => ['YOUR_WALLET_ADDRESS']]],
                                'outputs' => [['addresses' => [$toAddress], 'value' => $amount]],
                                'api_key' => $apiKey
                            ]
                        ]);
                    
                        // Conferma la transazione
                        $tx = json_decode($response->getBody(), true);
                        $sendResponse = $client->post("$apiUrl/send", [
                            'json' => $tx
                        ]);
                    
                        return json_decode($sendResponse->getBody(), true);
                    }
                    

  **Esempio di chiamata PHP**
  
                    $cryptoType = 'BTC';
                    $toAddress = '1A1zP1eP5QGefi2DM***************Na';
                    $amount = 10000;  // In satoshi (BTC) o unità specifica per altre criptovalute
                    $apiKey = 'YOUR_BLOCKCYPHER_API_KEY';
                    
                    $response = sendCryptoPayment($cryptoType, $toAddress, $amount, $apiKey);
                    print_r($response);

### **Logica delle API**

1. **API Endpoint di BlockCypher**:
   
   - **BTC**: `https://api.blockcypher.com/v1/btc/main/txs/new`
   - **LTC**: `https://api.blockcypher.com/v1/ltc/main/txs/new`
   - **DOGE**: `https://api.blockcypher.com/v1/doge/main/txs/new`
   - **DASH**: `https://api.blockcypher.com/v1/dash/main/txs/new`
   
3. **Chiamata API**:
   
   - Il backend invia una richiesta a BlockCypher per creare una nuova transazione.
   - L'indirizzo del wallet destinatario e l'importo sono forniti come parametri nella richiesta.

5. **Esecuzione della Transazione**:
6. 
   - Dopo aver creato la transazione, viene inviata una seconda richiesta per firmare e inviare la transazione sulla blockchain.

### **Flusso Completo**

1. **Contratto Smart su Ethereum**: lo smart contract chiama la funzione `sendCryptoOffChain`.
2. **Backend PHP**: riceve l'indirizzo della criptovaluta e l'importo dallo smart contract e fa la chiamata API per inviare il pagamento.
3. **BlockCypher API**: gestisce la transazione off-chain sulla blockchain della criptovaluta selezionata (ad esempio BTC, DOGE, ecc.).

### **Considerazioni Finali**

- Questo approccio integra il contratto **on-chain** su **Ethereum** con un backend **off-chain** che gestisce transazioni **multi-criptovaluta attraverso API esterne** come **BlockCypher**.
- È possibile adattare questa soluzione per supportare altre criptovalute o API come **Coinbase Commerce** o come le API delle **Mining-Pool** del caso in produzione oppure supportare direttamente i **nodi full per diverse blockchain**.
