### Hashburst Blockchain Framework (Python, PHP, Go)

In this document, it's provided the description of the **Hashburst Blockchain** framework with **Proof of History (PoH)** in three languages: Python, PHP, and Go. This blockchain uses **AES-256-CBC encryption** for user and wallet data, **SHA-512** for Proof of History, and **CRC32** for blockId-signature generation. The framework will have the following components:

1. **Ledger**: A collection of user data and wallet info stored in JSON files. Each file is encrypted with user-specific keys.
2. **Proof of History (PoH)**: Every block combines transaction data with a timestamp and is hashed using SHA-512 to create a proof of history.
3. **Consensus Algorithm**: Each block is validated based on the PoH mechanism, ensuring security and preventing double-spending.
4. **Blockchain Core**: The main blockchain file (`masterData.hbx`) stores all validated blocks and transactions, including the user and wallet information.
5. **Block Signature**: Generated using a combination of encryption methods and hash functions.

The **Hashburst structure** has built in **Python, PHP, and Go** for decentralized nodes.

---

### 1. **Python Version**

#### File Structure:
- `ledger/` → Contains user data files encrypted with API keys.
- `wallets/` → Contains encrypted wallet details.
- `blocks/` → Contains all voted changes and transactions between users.
- `masterData.hbx` → Encrypted blockchain data file containing public and private sections.

                  #### Python Code:
                  
                  import json
                  import hashlib
                  import time
                  from Crypto.Cipher import AES
                  from Crypto.Util.Padding import pad, unpad
                  import base64
                  import os
                  
                  # Encryption/Decryption function using AES-256-CBC
                  def encrypt_decrypt(data, key, iv, action):
                      key = hashlib.sha512(key.encode()).digest()[:32]
                      iv = hashlib.sha512(iv.encode()).digest()[:16]
                      cipher = AES.new(key, AES.MODE_CBC, iv)
                      
                      if action == 'encrypt':
                          encrypted = cipher.encrypt(pad(data.encode(), AES.block_size))
                          return base64.b64encode(encrypted).decode()
                      elif action == 'decrypt':
                          decrypted = unpad(cipher.decrypt(base64.b64decode(data)), AES.block_size)
                          return decrypted.decode()
                  
                  # Generate Proof of History
                  def generate_proof_of_history(data):
                      timestamp = time.time()
                      combined_data = data + str(timestamp)
                      proof = hashlib.sha512(combined_data.encode()).hexdigest()
                      
                      return {
                          'data': data,
                          'timestamp': timestamp,
                          'proof': proof

The complete Python version of the **Hashburst Blockchain** includes the core functions for encryption, decryption, Proof of History (PoH), user management, and block verification. Here's the continuation of the code and details for the entire framework.

---

#### Functions for Storing and Retrieving User Data
                          
                          # Save user data in the ledger
                          def save_user_data(user_id, data, api_key, password):
                              user_signature = hashlib.crc32(data.encode())
                              encrypted_data = encrypt_decrypt(data, api_key, password, 'encrypt')
                              user_data_path = f"ledger/{user_signature}"
                              
                              with open(user_data_path, 'w') as f:
                                  f.write(json.dumps({'data': encrypted_data, 'signature': user_signature}))
                              
                              return user_signature
                          
                          # Retrieve user data from ledger
                          def retrieve_user_data(user_signature, api_key, password):
                              user_data_path = f"ledger/{user_signature}"
                              
                              with open(user_data_path, 'r') as f:
                                  encrypted_data = json.load(f)['data']
                              
                              decrypted_data = encrypt_decrypt(encrypted_data, api_key, password, 'decrypt')
                              return decrypted_data


#### Creating Blocks and Handling Transactions
                        
                        # Create a new block and add it to masterData.hbx
                        def create_block(transaction_data, user_api_key, user_password):
                            proof = generate_proof_of_history(transaction_data)
                            block_signature = hashlib.crc32(proof['proof'].encode())
                        
                            # Encrypt the block data
                            encrypted_block = encrypt_decrypt(transaction_data, user_api_key, user_password, 'encrypt')
                        
                            # Append the encrypted block to the blockchain
                            master_data_path = 'blocks/masterData.hbx'
                            
                            with open(master_data_path, 'a') as f:
                                block = {
                                    'block_signature': block_signature,
                                    'encrypted_block': encrypted_block,
                                    'proof': proof
                                }
                                f.write(json.dumps(block) + '\n')
                            
                            return block_signature
                        
                        # Verify block with proof of history
                        def verify_block(block):
                            proof = generate_proof_of_history(block['data'])
                            return block['proof'] == proof['proof']

#### Transaction Handling
                        
                        # Process a transaction between two users
                        def process_transaction(sender_signature, receiver_signature, amount, coin_type):
                            transaction = {
                                'sender': sender_signature,
                                'receiver': receiver_signature,
                                'amount': amount,
                                'coin_type': coin_type,
                                'timestamp': time.time()
                            }
                            
                            # Add the transaction to the blockchain
                            block_signature = create_block(json.dumps(transaction), 'public_key', 'public_iv')
                            return block_signature

------------------------

### 2. **PHP Version**

PHP handles encryption and the blockchain structure similarly to the Python version. Here's a brief breakdown of the equivalent PHP code.

#### File Structure:
- `ledger/` → Contains user data in encrypted JSON files.
- `wallets/` → Contains encrypted wallet details.
- `blocks/` → Contains changes and transactions.
- `masterData.hbx` → Contains the blockchain.

#### PHP Code:

                        <?php
                        // AES-256-CBC encryption/decryption function
                        function encrypt_decrypt($string, $secret_key, $secret_iv, $action) {
                            $encrypt_method = "AES-256-CBC";
                            $key = hash('sha512', $secret_key);
                            $iv = substr(hash('sha512', $secret_iv), 0, 16);
                            
                            if ($action == 'encrypt') {
                                $output = openssl_encrypt($string, $encrypt_method, $key, 0, $iv);
                                $output = base64_encode($output);
                            } else if ($action == 'decrypt') {
                                $output = openssl_decrypt(base64_decode($string), $encrypt_method, $key, 0, $iv);
                            }
                            
                            return $output;
                        }
                        
                        // Generate Proof of History
                        function generateProofOfHistory($data) {
                            $timestamp = time();
                            $combinedData = $data . $timestamp;
                            $proof = hash('sha512', $combinedData);
                            
                            return array(
                                'data' => $data,
                                'timestamp' => $timestamp,
                                'proof' => $proof
                            );
                        }
                        
                        // Save user data to ledger
                        function saveUserData($userId, $data, $api_key, $password) {
                            $signature = hash('crc32b', $data);
                            $encryptedData = encrypt_decrypt($data, $api_key, $password, 'encrypt');
                            file_put_contents("ledger/$signature", json_encode(array('data' => $encryptedData, 'signature' => $signature)));
                            return $signature;
                        }
                        
                        // Create a new block and add to masterData.hbx
                        function createBlock($transactionData, $apiKey, $password) {
                            $proof = generateProofOfHistory($transactionData);
                            $blockSignature = hash('crc32b', $proof['proof']);
                            
                            $encryptedBlock = encrypt_decrypt($transactionData, $apiKey, $password, 'encrypt');
                            
                            $block = array(
                                'block_signature' => $blockSignature,
                                'encrypted_block' => $encryptedBlock,
                                'proof' => $proof
                            );
                            
                            file_put_contents('blocks/masterData.hbx', json_encode($block) . PHP_EOL, FILE_APPEND);
                            return $blockSignature;
                        }
                        ?>

------------------------

### 3. **Go Version**

The **Go version** will compile into an executable (`hashburstCore`). It will follow the same structure as the Python and PHP versions but include concurrency for peer-to-peer nodes and transaction handling.

#### Go Code (Basic Structure):

                        package main
                        
                        import (
                            "crypto/aes"
                            "crypto/cipher"
                            "crypto/sha512"
                            "encoding/base64"
                            "fmt"
                            "time"
                        )
                        
                        // Encrypt and decrypt function using AES-256-CBC
                        func encryptDecrypt(data, key, iv string, action string) (string, error) {
                            hashKey := sha512.Sum512([]byte(key))
                            hashIv := sha512.Sum512([]byte(iv))
                        
                            block, err := aes.NewCipher(hashKey[:32])
                            if err != nil {
                                return "", err
                            }
                        
                            if action == "encrypt" {
                                ciphertext := make([]byte, len(data))
                                cipher.NewCBCEncrypter(block, hashIv[:16]).CryptBlocks(ciphertext, []byte(data))
                                return base64.StdEncoding.EncodeToString(ciphertext), nil
                            } else {
                                decodedData, err := base64.StdEncoding.DecodeString(data)
                                if err != nil {
                                    return "", err
                                }
                        
                                plaintext := make([]byte, len(decodedData))
                                cipher.NewCBCDecrypter(block, hashIv[:16]).CryptBlocks(plaintext, decodedData)
                                return string(plaintext), nil
                            }
                        }
                        
                        // Generate Proof of History
                        func generateProofOfHistory(data string) (string, string) {
                            timestamp := time.Now().Unix()
                            combinedData := fmt.Sprintf("%s%d", data, timestamp)
                            proof := sha512.Sum512([]byte(combinedData))
                            return string(proof[:]), fmt.Sprintf("%d", timestamp)
                        }

The Go version will include additional P2P networking and validation, enabling decentralized nodes and interactions. Nodes can communicate over TCP or over TEP[^x], sending encrypted transactions and blocks.

------------------------

### Summary of Framework Structure:

1. **Ledger**: Stores user and wallet data, encrypted with user-specific keys.
2. **PoH Mechanism**: Ensures tamper-proof transactions by adding a timestamp and hashing the data with SHA-512.
3. **Master Chain (masterData.hbx)**: Stores all transactions and changes validated by consensus.
4. **Private/Public Data**: Users can access only their data, while public information is accessible to all nodes.
5. **Decentralized Nodes**: Go version enables peer-to-peer communication for decentralized validation.

This framework has been extended with P2P capabilities, smart contracts, and voting/consensus system for all nodes in the main network (ecosystem).

[^x]: Hashburst can also run on protocols other than TCP/IP such as, for example, the TEP protocol. The patents and can be seen at the following links on the WIPO website: - [US20210243031](https://patentscope.wipo.int/search/en/detail.jsf?docId=US332615987&_cid=P20-M0ZG2C-76315-1); - [IN202017051650](https://patentscope.wipo.int/search/en/detail.jsf?docId=IN318462466&_cid=P20-M0ZG2C-76315-1); - [IT201800005763](https://patentscope.wipo.int/search/en/detail.jsf?docId=IT294324396&_cid=P20-M0ZG2C-76315-1); - [EP3804376](https://patentscope.wipo.int/search/en/detail.jsf?docId=EP321940846&_cid=P20-M0ZG2C-76315-1); - [WO2019229612](https://patentscope.wipo.int/search/en/detail.jsf?docId=WO2019229612&_cid=P20-M0ZG2C-76315-1); - [US20240028421](https://patentscope.wipo.int/search/en/detail.jsf?docId=US420408951&_cid=P20-M0ZG2C-76315-1); - [US20230239155](https://patentscope.wipo.int/search/en/detail.jsf?docId=US403068095&_cid=P20-M0ZG2C-76315-1); - [US20230231836](https://patentscope.wipo.int/search/en/detail.jsf?docId=US402828261&_cid=P20-M0ZG2C-76315-1); - [EP4311159](https://patentscope.wipo.int/search/en/detail.jsf?docId=EP420424266&_cid=P20-M0ZG2C-76315-1); - [IT202200015489](https://patentscope.wipo.int/search/en/detail.jsf?docId=IT436939565&_cid=P20-M0ZG2C-76315-1); - [IT202000014509](https://patentscope.wipo.int/search/en/detail.jsf?docId=IT397727902&_cid=P20-M0ZG2C-76315-2); - [EP4169233](https://patentscope.wipo.int/search/en/detail.jsf?docId=EP396253009&_cid=P20-M0ZG2C-76315-2); - [IT202000014518](https://patentscope.wipo.int/search/en/detail.jsf?docId=IT397727906&_cid=P20-M0ZG2C-76315-2); - [EP4169234](https://patentscope.wipo.int/search/en/detail.jsf?docId=EP396253010&_cid=P20-M0ZG2C-76315-2); - [WO2021255630](https://patentscope.wipo.int/search/en/detail.jsf?docId=WO2021255630&_cid=P20-M0ZG2C-76315-2); - [WO2021255633](https://patentscope.wipo.int/search/en/detail.jsf?docId=WO2021255633&_cid=P20-M0ZG2C-76315-2)
