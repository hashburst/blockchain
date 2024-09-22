<?php

// Esempio di chiamata
$cryptoType = 'BTC';
$toAddress = '1A1zP1eP5QGe***************Na';
$amount = 10000;  // In satoshi (BTC) o unità specifica per altre criptovalute
$apiKey = 'YOUR_BLOCKCYPHER_API_KEY';

$response = sendCryptoPayment($cryptoType, $toAddress, $amount, $apiKey);
print_r($response);

?>
