<?php
  // API PHP Front Controller

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

?>
