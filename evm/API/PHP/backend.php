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

?>
