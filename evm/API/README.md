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

                  php
                  
                  if ($_GET['action'] === 'mint') {
                      // Process mint request here
                      $method = 'eth_sendTransaction';
                      // Call smart contract mint function
                  }

#### **2. Transfer Tokens**

This API handles token transfers from one user to another. It can work for both fungible (HBT-20) and non-fungible (HBT-721) tokens.

                  php
                  
                  if ($_GET['action'] === 'transfer') {
                      // Process token transfer
                      $method = 'eth_sendTransaction';
                      // Call smart contract transfer function
                  }

#### **3. Check Token Balance**

This endpoint checks the balance of a given user’s wallet, allowing third-party systems to get real-time token holdings.

                  php
                  
                  if ($_GET['action'] === 'balance') {
                      // Query balance using eth_call
                      $method = 'eth_call';
                      // Fetch the balance from smart contract
                  }

#### **4. Auto Withdrawal Request for Pools**

This is the most complex part, where the system automates payments based on user roles and contributions. **Dealer** and **Reseller** wallets receive predefined percentages from the gross mined amount, while the rest is split among users based on their accepted shares.

- **Dealer Share**: 5% of the gross mined amount.
- **Reseller Share**: 2% of the gross mined amount.
- **User Share**: The remaining amount is distributed proportionally based on the user's accepted shares from the pool.

The system sends the relevant amount to each wallet using smart contract transactions:

                  php
                  
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
