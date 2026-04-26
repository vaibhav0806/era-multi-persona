// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

// MockPublicResolver — minimal stand-in for ENS PublicResolver. Implements
// only setText / text. Function signatures match the real PublicResolver at
// 0xE99638b40E4Fff0129D56f03b55b6bbC4BBE49b5 on Sepolia.
contract MockPublicResolver {
    mapping(bytes32 => mapping(string => string)) private _texts;

    function setText(bytes32 node, string calldata key, string calldata value) external {
        _texts[node][key] = value;
    }

    function text(bytes32 node, string calldata key) external view returns (string memory) {
        return _texts[node][key];
    }
}
