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
    event PaymentRequested(string cryptoAddress, uint256 amount, string cryptoType);
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

    // Funzione per inviare pagamenti off-chain tramite API esterne (come BlockCypher)
    function sendCryptoOffChain(string memory _cryptoAddress, uint256 _amount, string memory _cryptoType) private {
        // Lancia un evento che il backend può ascoltare per inviare il pagamento
        emit PaymentRequested(_cryptoAddress, _amount, _cryptoType);
        // Il backend (PHP con Guzzle) cattura l'evento e invia il pagamento tramite l'API di BlockCypher
    }

    receive() external payable {}
}
