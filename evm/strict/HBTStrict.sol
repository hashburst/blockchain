// SPDX-License-Identifier: MIT
pragma solidity 0.8.30;
import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Permit.sol";
import "@openzeppelin/contracts/token/ERC721/extensions/ERC721URIStorage.sol";
import "@openzeppelin/contracts/token/common/ERC2981.sol";
import "@openzeppelin/contracts/token/ERC1155/extensions/ERC1155Supply.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC4626.sol";
import "@openzeppelin/contracts/interfaces/IERC1271.sol";
import "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";

// HBT labels are deployment profiles. Inherited Ethereum selectors/events and
// interface IDs remain unchanged. These tokens are not native HBT gas currency.
contract HBT20Strict is ERC20, ERC20Permit {
 constructor() ERC20("Strict Test Asset","STA") ERC20Permit("Strict Test Asset") { _mint(msg.sender,1_000_000 * 10**18); }
}
contract HBT721Strict is ERC721URIStorage, ERC2981 {
 constructor() ERC721("Strict Test NFT","STN") { _mint(msg.sender,1); _setTokenURI(1,"ipfs://example"); _setDefaultRoyalty(msg.sender,500); }
 function supportsInterface(bytes4 id) public view override(ERC721URIStorage,ERC2981) returns(bool) {return super.supportsInterface(id);}
}
contract HBT1155Strict is ERC1155Supply {
 constructor() ERC1155("ipfs://example/{id}.json") { _mint(msg.sender,1,100,""); }
}
contract HBT4626Strict is ERC4626 {
 constructor(IERC20 asset_) ERC20("Strict Test Vault","STV") ERC4626(asset_) {}
}
contract HBT1271Strict is IERC1271 {
 address public immutable owner;
 constructor() {owner=msg.sender;}
 function isValidSignature(bytes32 hash,bytes memory signature) external view returns(bytes4) {
  (address signer,ECDSA.RecoverError error,)=ECDSA.tryRecover(hash,signature);
  return error==ECDSA.RecoverError.NoError && signer==owner ? IERC1271.isValidSignature.selector : bytes4(0xffffffff);
 }
}
