<?php

    require 'vendor/autoload.php';  // Guzzle e Web3 PHP
    
    use GuzzleHttp\Client;
    use Web3\Web3;
    use Web3\Contract;
    
    // Configura il client Web3 per connettersi a Ethereum
    $web3 = new Web3('http://localhost:8545');  // Cambiare per il proprio nodo Ethereum
    $contract = new Contract($web3->provider, 'ABI del contratto');
    
    // Ascolta l'evento PaymentRequested dal contratto
    $contract->at('0xIndirizzoContratto')->on('PaymentRequested', function ($error, $event) {
        if ($error !== null) {
            echo 'Errore durante l'ascolto dell'evento: ' . $error->getMessage();
            return;
        }
    
        // Ottieni i parametri dell'evento
        $cryptoAddress = $event->data['cryptoAddress'];
        $amount = $event->data['amount'];
        $cryptoType = $event->data['cryptoType'];
    
        // Invia il pagamento tramite BlockCypher
        sendCryptoPayment($cryptoType, $cryptoAddress, $amount);
    });
    
    // Funzione per inviare il pagamento tramite l'API di BlockCypher
    function sendCryptoPayment($cryptoType, $toAddress, $amount) {
        $client = new Client();
        $apiUrl = "https://api.blockcypher.com/v1/";
    
        // Determina l'URL API corretto in base alla criptovaluta
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
    
        try {
            // Crea la transazione
            $response = $client->post($apiUrl, [
                'json' => [
                    'inputs' => [['addresses' => ['YOUR_WALLET_ADDRESS']]],
                    'outputs' => [['addresses' => [$toAddress], 'value' => $amount]],
                    'api_key' => 'YOUR_BLOCKCYPHER_API_KEY'
                ]
            ]);
    
            // Conferma la transazione
            $tx = json_decode($response->getBody(), true);
            $sendResponse = $client->post("$apiUrl/send", [
                'json' => $tx
            ]);
    
            $result = json_decode($sendResponse->getBody(), true);
            echo "Transaction sent: TXID = " . $result['tx']['hash'] . "\n";
        } catch (Exception $e) {
            echo "Error: " . $e->getMessage() . "\n";
        }
    }

?>
