// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/KlingonHTLC.sol";

contract DeployKlingonHTLC is Script {
    // DAO addresses per network
    // TODO: Replace with actual mainnet DAO addresses before production
    function getDAOAddress() internal view returns (address) {
        uint256 chainId = block.chainid;

        // Testnets
        if (chainId == 11155111) return 0x37e565Bab0c11756806480102E09871f33403D8d; // Sepolia
        if (chainId == 97) return 0x37e565Bab0c11756806480102E09871f33403D8d;       // BSC Testnet
        if (chainId == 80001) return 0x37e565Bab0c11756806480102E09871f33403D8d;    // Polygon Mumbai
        if (chainId == 421614) return 0x37e565Bab0c11756806480102E09871f33403D8d;   // Arbitrum Sepolia

        // Mainnets - TODO: Set actual DAO addresses
        if (chainId == 1) revert("Mainnet DAO address not set");     // Ethereum
        if (chainId == 56) revert("BSC DAO address not set");        // BSC
        if (chainId == 137) revert("Polygon DAO address not set");   // Polygon
        if (chainId == 42161) revert("Arbitrum DAO address not set"); // Arbitrum

        revert("Unsupported chain");
    }

    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        address daoAddress = getDAOAddress();

        console.log("Deploying KlingonHTLC...");
        console.log("Chain ID:", block.chainid);
        console.log("Deployer:", deployer);
        console.log("DAO Address:", daoAddress);

        vm.startBroadcast(deployerPrivateKey);

        KlingonHTLC htlc = new KlingonHTLC(daoAddress);

        vm.stopBroadcast();

        console.log("KlingonHTLC deployed at:");
        console.logAddress(address(htlc));
    }
}

contract SetFee is Script {
    function run(address htlcAddress, uint256 newFeeBps) external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");

        vm.startBroadcast(deployerPrivateKey);

        KlingonHTLC htlc = KlingonHTLC(htlcAddress);
        htlc.setFeeBps(newFeeBps);

        vm.stopBroadcast();

        console.log("Fee updated to:", newFeeBps, "bps");
    }
}

contract SetDAOAddress is Script {
    function run(address htlcAddress, address newDaoAddress) external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");

        vm.startBroadcast(deployerPrivateKey);

        KlingonHTLC htlc = KlingonHTLC(htlcAddress);
        htlc.setDaoAddress(newDaoAddress);

        vm.stopBroadcast();

        console.log("DAO address updated to:", newDaoAddress);
    }
}

contract PauseContract is Script {
    function run(address htlcAddress, bool pause) external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");

        vm.startBroadcast(deployerPrivateKey);

        KlingonHTLC htlc = KlingonHTLC(htlcAddress);
        htlc.setPaused(pause);

        vm.stopBroadcast();

        console.log("Contract paused:", pause);
    }
}
