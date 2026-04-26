// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

// MockNameWrapper — minimal stand-in for ENS NameWrapper used in unit tests.
// Implements ONLY the subset of methods era-brain/identity/ens.Provider calls:
// - setSubnodeRecord(parentNode, label, owner, resolver, ttl, fuses, expiry)
// - ownerOf(tokenId)
//
// Function signatures (parameter types + names) MUST match the real
// NameWrapper at 0x0635513f179D50A207757E05759CbD106d7dFcE8 on Sepolia, so
// abigen output works against both.
contract MockNameWrapper {
    // tokenId = uint256(node) where node is the ENS namehash bytes32.
    mapping(uint256 => address) private _owners;

    // Mint helper for tests: register parentNode as owned by msg.sender.
    function testMint(bytes32 parentNode, address to) external {
        _owners[uint256(parentNode)] = to;
    }

    function ownerOf(uint256 tokenId) external view returns (address) {
        return _owners[tokenId];
    }

    function setSubnodeRecord(
        bytes32 parentNode,
        string calldata label,
        address owner,
        address /* resolver */,
        uint64 /* ttl */,
        uint32 /* fuses */,
        uint64 /* expiry */
    ) external returns (bytes32 node) {
        require(_owners[uint256(parentNode)] == msg.sender, "MockNameWrapper: not owner of parent");
        node = keccak256(abi.encodePacked(parentNode, keccak256(bytes(label))));
        _owners[uint256(node)] = owner;
        return node;
    }
}
