# Wallet nativi del fondatore

Il comando hvm-wallet-init crea conti secp256k1 con indirizzo 0x compatibile con
il wallet nativo HashBurst. Non richiede MetaMask. Un indirizzo non distingue da
solo coin e token: il coin HBT e contabilizzato dal protocollo, i token dai contratti.
asset_type=native_coin nel descrittore pubblico e un'etichetta del file, non una
modifica al protocollo o un'allocazione di saldo.

create-wallets.sh genera sul Mac due chiavi diverse in:
- ~/.hashburst-native-wallets/testnet-4735490/wallet.key
- ~/.hashburst-native-wallets/mainnet-4735489/wallet.key

Le directory sono 0700 e i file 0600. Le chiavi sono conservate in esadecimale,
non cifrate dal programma: custodire una copia offline protetta. Non inviare
wallet.key, non inserirlo in Git e non copiarlo sui validatori. Si possono condividere
solo gli indirizzi e i file public.json. Il programma non genera una seed phrase.

Una seconda esecuzione conserva le identita. Se manca una chiave ma esiste gia
il descrittore pubblico, si ferma: non sostituisce un wallet che potrebbe avere fondi.
I due conti appartengono a domini di firma distinti tramite le chain ID, ma il
formato 0x in se non codifica la rete. Le chiavi distinte evitano il riuso locale.

Il miliardo di HBT approvato per il fondatore mainnet NON viene accreditato da
questo strumento. Richiede un manifest di allocazione separato con questo indirizzo,
la contabilizzazione dell'eventuale import legacy e la verifica di conservazione
monetaria. Nessun saldo testnet viene copiato sulla mainnet.
