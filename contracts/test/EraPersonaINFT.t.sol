// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "../src/EraPersonaINFT.sol";

contract EraPersonaINFTTest is Test {
    EraPersonaINFT internal inft;
    address internal owner;
    address internal stranger;
    address internal holder;

    function setUp() public {
        owner = address(this); // test contract is deployer
        stranger = makeAddr("stranger");
        holder = makeAddr("holder");
        inft = new EraPersonaINFT(owner);
    }

    // Required because some tests mint to address(this) — _safeMint calls
    // onERC721Received on contract recipients. forge-std's Test contract
    // doesn't implement IERC721Receiver, so we provide it here.
    function onERC721Received(address, address, uint256, bytes calldata)
        external pure returns (bytes4)
    {
        return this.onERC721Received.selector;
    }

    // ---- mint + tokenURI tests (Phase 1) ----

    function testMintByOwner() public {
        uint256 tokenId = inft.mint(owner, "ipfs://planner.json");
        assertEq(tokenId, 0, "first tokenId should be 0");
        assertEq(inft.ownerOf(0), owner);
        assertEq(inft.tokenURI(0), "ipfs://planner.json");
        assertEq(inft.totalSupply(), 1);
    }

    function testMintIncrementsTokenId() public {
        inft.mint(owner, "ipfs://planner.json");
        uint256 second = inft.mint(owner, "ipfs://coder.json");
        assertEq(second, 1, "second tokenId should be 1");
        assertEq(inft.totalSupply(), 2);
    }

    function testMintByNonOwnerReverts() public {
        vm.prank(stranger);
        vm.expectRevert(); // OZ Ownable v5 reverts with OwnableUnauthorizedAccount(address)
        inft.mint(stranger, "ipfs://malicious.json");
    }

    function testTokenURIRevertsForNonExistent() public {
        vm.expectRevert(); // ERC721 v5 reverts with ERC721NonexistentToken(uint256)
        inft.tokenURI(999);
    }

    function testMintToDifferentRecipient() public {
        uint256 tokenId = inft.mint(holder, "ipfs://planner.json");
        assertEq(inft.ownerOf(tokenId), holder, "holder owns the token even though owner minted");
        assertEq(inft.balanceOf(holder), 1);
        assertEq(inft.balanceOf(owner), 0);
    }
}
