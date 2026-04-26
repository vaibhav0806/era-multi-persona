// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "openzeppelin-contracts/contracts/token/ERC721/ERC721.sol";
import "openzeppelin-contracts/contracts/access/Ownable.sol";

/// @title EraPersonaINFT
/// @notice ERC-7857-inspired minimal iNFT for era's coding-agent personas.
///         Each token represents one persona (planner/coder/reviewer + future user-mints).
///         tokenURI points at a JSON blob describing the persona.
/// @dev Out-of-scope vs full ERC-7857: encrypted-metadata transfer, clone(),
///      authorizeUsage(), TEE/ZKP oracles, royalty splits. Roadmap items.
contract EraPersonaINFT is ERC721, Ownable {
    uint256 private _nextTokenId;
    mapping(uint256 => string) private _tokenURIs;

    /// @notice Emitted when an iNFT is invoked (per sealed-inference run).
    /// @param tokenId The persona invoked.
    /// @param receiptHash sha256 of the brain.Receipt (32 bytes).
    /// @param ts block.timestamp at the time of recording.
    event Invocation(uint256 indexed tokenId, bytes32 indexed receiptHash, uint256 indexed ts);

    constructor(address initialOwner) ERC721("Era Persona iNFT", "ERAINFT") Ownable(initialOwner) {}

    /// @notice Mint a new persona iNFT. Only contract owner.
    /// @param to Recipient.
    /// @param uri Metadata URI (raw GitHub URL of persona JSON for hackathon scope).
    /// @return tokenId The newly minted token's ID.
    function mint(address to, string memory uri) external onlyOwner returns (uint256 tokenId) {
        tokenId = _nextTokenId++;
        _safeMint(to, tokenId);
        _tokenURIs[tokenId] = uri;
    }

    /// @notice Get the metadata URI for a token. Reverts on non-existent token.
    function tokenURI(uint256 tokenId) public view override returns (string memory) {
        _requireOwned(tokenId);
        return _tokenURIs[tokenId];
    }

    function totalSupply() external view returns (uint256) {
        return _nextTokenId;
    }

    /// @notice Record a persona-invocation event on-chain. Callable by the
    ///         contract owner OR the current token holder.
    /// @param tokenId The persona invoked.
    /// @param receiptHash sha256 of the run's Receipt struct.
    function recordInvocation(uint256 tokenId, bytes32 receiptHash) external {
        require(_ownerOf(tokenId) != address(0), "EraPersonaINFT: token does not exist");
        require(
            msg.sender == owner() || msg.sender == _ownerOf(tokenId),
            "EraPersonaINFT: not authorized"
        );
        emit Invocation(tokenId, receiptHash, block.timestamp);
    }
}
