<?php
  
  // Funzione principale per distribuire i fondi ai wallet degli utenti
  function distributeFunds($grossMinedAmount, $blockIdSignature) {
      $wallets = getWalletsFromBlockId($blockIdSignature); // Recupera i wallet dal file associato
      $dealerShare = $grossMinedAmount * 0.05;  // Nell'ipotesi di progetto, 5% al dealer
      $resellerShare = $grossMinedAmount * 0.02;  // Nell'ipotesi di progetto, 2% al reseller
      $netMinedAmount = $grossMinedAmount - $dealerShare - $resellerShare;
  
      // Invio dei fondi ai wallet degli utenti
      foreach ($wallets as $crypto => $walletAddress) {
          if ($crypto == 'DOGE') {
              sendDogePayment($walletAddress, calculateShare($netMinedAmount, $crypto));
          } elseif ($crypto == 'BTC') {
              sendBtcPayment($walletAddress, calculateShare($netMinedAmount, $crypto));
          } elseif ($crypto == 'BCH') {
              sendBchPayment($walletAddress, calculateShare($netMinedAmount, $crypto));
          } elseif ($crypto == 'LTC') {
              sendLtcPayment($walletAddress, calculateShare($netMinedAmount, $crypto));
          } elseif ($crypto == 'ETC') {
              sendEtcPayment($walletAddress, calculateShare($netMinedAmount, $crypto));
          } elseif ($crypto == 'XMR') {
              sendXmrPayment($walletAddress, calculateShare($netMinedAmount, $crypto));
          } elseif ($crypto == 'DASH') {
              sendDashPayment($walletAddress, calculateShare($netMinedAmount, $crypto));
          }
      }
  }
  
  // Funzione per inviare Dogecoin
  function sendDogePayment($walletAddress, $amount) {
      // Usa Blockcypher o un nodo Dogecoin per inviare il pagamento
      sendCryptoPayment('DOGE', $walletAddress, $amount);
  }
  
  // Funzione per inviare Bitcoin
  function sendBtcPayment($walletAddress, $amount) {
      // Usa Blockcypher o un nodo Bitcoin per inviare il pagamento
      sendCryptoPayment('BTC', $walletAddress, $amount);
  }

  // Funzione per inviare Bitcoin Cash
  function sendBchPayment($walletAddress, $amount) {
      // Usa Blockcypher o un nodo Bitcoin Cash per inviare il pagamento
      sendCryptoPayment('BCH', $walletAddress, $amount);
  }

  // Funzione per inviare Litecoin
  function sendLtcPayment($walletAddress, $amount) {
      // Usa Blockcypher o un nodo Litecoin per inviare il pagamento
      sendCryptoPayment('LTC', $walletAddress, $amount);
  }

  // Funzione per inviare Ether Classic
  function sendEtcPayment($walletAddress, $amount) {
      // Usa Blockcypher o un nodo Ether Classic per inviare il pagamento
      sendCryptoPayment('ETC', $walletAddress, $amount);
  }

  // Funzione per inviare Monero
  function sendXmrPayment($walletAddress, $amount) {
      // Usa Blockcypher o un nodo Monero per inviare il pagamento
      sendCryptoPayment('XMR', $walletAddress, $amount);
  }

  // Funzione per inviare Dash Coin
  function sendDashPayment($walletAddress, $amount) {
      // Usa Blockcypher o un nodo Dash Coin per inviare il pagamento
      sendCryptoPayment('DASH', $walletAddress, $amount);
  }

  // Funzione generica per inviare pagamenti tramite Blockcypher
  function sendCryptoPayment($cryptoType, $walletAddress, $amount) {
      // Integra l'API di BlockCypher per inviare la transazione
      $client = new GuzzleHttp\Client();
      $apiUrl = "https://api.blockcypher.com/v1/{$cryptoType}/main/txs/new";
      $apiKey = 'YOUR_API_KEY';
  
      try {
          $response = $client->post($apiUrl, [
              'json' => [
                  'inputs' => [['addresses' => ['YOUR_WALLET_ADDRESS']]],
                  'outputs' => [['addresses' => [$walletAddress], 'value' => $amount]],
                  'api_key' => $apiKey
              ]
          ]);
  
          $tx = json_decode($response->getBody(), true);
          $sendResponse = $client->post("{$apiUrl}/send", [
              'json' => $tx
          ]);
  
          $result = json_decode($sendResponse->getBody(), true);
          echo "Transaction sent: TXID = " . $result['tx']['hash'] . "\n";
      } catch (Exception $e) {
          echo "Error: " . $e->getMessage() . "\n";
      }
  }

  // Funzione per recuperare i Wallet dal BlockId-Signature
  function getWalletsFromBlockId($blockIdSignature) {
      $filePath = __DIR__ . "/blockchain/ledger/wallets/{$blockIdSignature}";
      $wallets = [];
  
      if (file_exists($filePath)) {
          $fileContents = file($filePath, FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES);
          foreach ($fileContents as $line) {
              list($crypto, $walletAddress) = explode(':', $line);
              $wallets[trim($crypto)] = trim($walletAddress);
          }
      }                  
      return $wallets;
  }

?>
