<?php
  require 'vendor/autoload.php'; // web3.php autoloader
  
  use Web3\Web3;
  use Web3\Contract;
  use Web3\Utils;
  
  $web3 = new Web3('http://localhost:8545'); // Connect to your EVM node
  $eth = $web3->eth;
  
  $eth->blockNumber(function ($err, $block) {
      if ($err !== null) {
          // Handle the error
          echo 'Error: ' . $err->getMessage();
          return;
      }
      // Output current block number
      echo 'Block number: ' . $block . PHP_EOL;
  });
  
  // Example: Deploy a contract (ensure your compiled contract is in bytecode format)
  $contract = new Contract($web3->provider, '0xCONTRACT_BYTECODE');
  
  // Interact with the contract
  $contract->at('0xCONTRACT_ADDRESS')->call('methodName', function ($err, $result) {
      if ($err !== null) {
          echo 'Error: ' . $err->getMessage();
          return;
      }
      echo 'Result: ' . json_encode($result) . PHP_EOL;
  });
?>
