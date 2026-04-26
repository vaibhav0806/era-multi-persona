// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/EraPersonaINFT.sol";

/// @notice Deploys EraPersonaINFT to 0G Galileo testnet and mints 3 default
///         personas (planner=0, coder=1, reviewer=2). Reads PI_ZG_PRIVATE_KEY
///         from env (forge-std vm.envUint accepts 0x-prefixed hex).
///
/// Usage:
///   set -a; source ../.env; set +a
///   forge script script/Deploy.s.sol --broadcast --rpc-url $PI_ZG_EVM_RPC --legacy
contract Deploy is Script {
    function run() external {
        uint256 deployerKey = vm.envUint("PI_ZG_PRIVATE_KEY");
        address deployer = vm.addr(deployerKey);

        // Raw GitHub URL base — points at master branch JSON files we committed
        // under contracts/metadata/.
        string memory baseURL = "https://raw.githubusercontent.com/vaibhav0806/era-multi-persona/master/contracts/metadata/";

        vm.startBroadcast(deployerKey);

        EraPersonaINFT inft = new EraPersonaINFT(deployer);

        uint256 plannerId  = inft.mint(deployer, string.concat(baseURL, "planner.json"));
        uint256 coderId    = inft.mint(deployer, string.concat(baseURL, "coder.json"));
        uint256 reviewerId = inft.mint(deployer, string.concat(baseURL, "reviewer.json"));

        vm.stopBroadcast();

        console.log("=== EraPersonaINFT deployed ===");
        console.log("Contract address:", address(inft));
        console.log("Planner tokenId :", plannerId);
        console.log("Coder tokenId   :", coderId);
        console.log("Reviewer tokenId:", reviewerId);
        console.log("Owner           :", deployer);
        console.log("");
        console.log("Add to .env:");
        console.log(string.concat("PI_ZG_INFT_CONTRACT_ADDRESS=", vm.toString(address(inft))));
    }
}
