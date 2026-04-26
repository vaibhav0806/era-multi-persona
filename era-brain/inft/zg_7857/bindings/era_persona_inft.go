// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package bindings

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// EraPersonaINFTMetaData contains all meta data concerning the EraPersonaINFT contract.
var EraPersonaINFTMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"initialOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"approve\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"balanceOf\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApproved\",\"inputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isApprovedForAll\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"mint\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"uri\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"name\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"ownerOf\",\"inputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"recordInvocation\",\"inputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"receiptHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"safeTransferFrom\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"safeTransferFrom\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setApprovalForAll\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"approved\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"symbol\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tokenURI\",\"inputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalSupply\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferFrom\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"Approval\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"approved\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ApprovalForAll\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"approved\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Invocation\",\"inputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"receiptHash\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"ts\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Transfer\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ERC721IncorrectOwner\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC721InsufficientApproval\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ERC721InvalidApprover\",\"inputs\":[{\"name\":\"approver\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC721InvalidOperator\",\"inputs\":[{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC721InvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC721InvalidReceiver\",\"inputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC721InvalidSender\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC721NonexistentToken\",\"inputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
	Bin: "0x608060405234801562000010575f80fd5b506040516200154b3803806200154b833981016040819052620000339162000131565b806040518060400160405280601081526020016f115c984814195c9cdbdb98481a53919560821b815250604051806040016040528060078152602001661154905253919560ca1b815250815f90816200008d9190620001fe565b5060016200009c8282620001fe565b5050506001600160a01b038116620000cd57604051631e4fbdf760e01b81525f600482015260240160405180910390fd5b620000d881620000e0565b5050620002ca565b600680546001600160a01b038381166001600160a01b0319831681179093556040519116919082907f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0905f90a35050565b5f6020828403121562000142575f80fd5b81516001600160a01b038116811462000159575f80fd5b9392505050565b634e487b7160e01b5f52604160045260245ffd5b600181811c908216806200018957607f821691505b602082108103620001a857634e487b7160e01b5f52602260045260245ffd5b50919050565b601f821115620001f957805f5260205f20601f840160051c81016020851015620001d55750805b601f840160051c820191505b81811015620001f6575f8155600101620001e1565b50505b505050565b81516001600160401b038111156200021a576200021a62000160565b62000232816200022b845462000174565b84620001ae565b602080601f83116001811462000268575f8415620002505750858301515b5f19600386901b1c1916600185901b178555620002c2565b5f85815260208120601f198616915b82811015620002985788860151825594840194600190910190840162000277565b5085821015620002b657878501515f19600388901b60f8161c191681555b505060018460011b0185555b505050505050565b61127380620002d85f395ff3fe608060405234801561000f575f80fd5b506004361061011c575f3560e01c806370a08231116100a9578063b88d4fde1161006e578063b88d4fde14610242578063c87b56dd14610255578063d0def52114610268578063e985e9c51461027b578063f2fde38b1461028e575f80fd5b806370a08231146101fb578063715018a61461020e5780638da5cb5b1461021657806395d89b4114610227578063a22cb4651461022f575f80fd5b806318160ddd116100ef57806318160ddd1461019d57806323b872dd146101af5780632f625088146101c257806342842e0e146101d55780636352211e146101e8575f80fd5b806301ffc9a71461012057806306fdde0314610148578063081812fc1461015d578063095ea7b314610188575b5f80fd5b61013361012e366004610d81565b6102a1565b60405190151581526020015b60405180910390f35b6101506102f2565b60405161013f9190610de6565b61017061016b366004610df8565b610381565b6040516001600160a01b03909116815260200161013f565b61019b610196366004610e2a565b6103a8565b005b6007545b60405190815260200161013f565b61019b6101bd366004610e52565b6103b7565b61019b6101d0366004610e8b565b610445565b61019b6101e3366004610e52565b610561565b6101706101f6366004610df8565b610580565b6101a1610209366004610eab565b61058a565b61019b6105cf565b6006546001600160a01b0316610170565b6101506105e2565b61019b61023d366004610ec4565b6105f1565b61019b610250366004610f84565b6105fc565b610150610263366004610df8565b610614565b6101a1610276366004610ffb565b6106bb565b610133610289366004611059565b610701565b61019b61029c366004610eab565b61072e565b5f6001600160e01b031982166380ac58cd60e01b14806102d157506001600160e01b03198216635b5e139f60e01b145b806102ec57506301ffc9a760e01b6001600160e01b03198316145b92915050565b60605f80546103009061108a565b80601f016020809104026020016040519081016040528092919081815260200182805461032c9061108a565b80156103775780601f1061034e57610100808354040283529160200191610377565b820191905f5260205f20905b81548152906001019060200180831161035a57829003601f168201915b5050505050905090565b5f61038b8261076b565b505f828152600460205260409020546001600160a01b03166102ec565b6103b38282336107a3565b5050565b6001600160a01b0382166103e557604051633250574960e11b81525f60048201526024015b60405180910390fd5b5f6103f18383336107b0565b9050836001600160a01b0316816001600160a01b03161461043f576040516364283d7b60e01b81526001600160a01b03808616600483015260248201849052821660448201526064016103dc565b50505050565b5f828152600260205260409020546001600160a01b03166104b45760405162461bcd60e51b8152602060048201526024808201527f457261506572736f6e61494e46543a20746f6b656e20646f6573206e6f7420656044820152631e1a5cdd60e21b60648201526084016103dc565b6006546001600160a01b03163314806104e257505f828152600260205260409020546001600160a01b031633145b61052e5760405162461bcd60e51b815260206004820152601e60248201527f457261506572736f6e61494e46543a206e6f7420617574686f72697a6564000060448201526064016103dc565b4281837f5299a77d2b4293488895b6dea81d075c8c010c488358ccd3c58c0ada2070eac860405160405180910390a45050565b61057b83838360405180602001604052805f8152506105fc565b505050565b5f6102ec8261076b565b5f6001600160a01b0382166105b4576040516322718ad960e21b81525f60048201526024016103dc565b506001600160a01b03165f9081526003602052604090205490565b6105d76108a2565b6105e05f6108cf565b565b6060600180546103009061108a565b6103b3338383610920565b6106078484846103b7565b61043f33858585856109e7565b606061061f8261076b565b505f82815260086020526040902080546106389061108a565b80601f01602080910402602001604051908101604052809291908181526020018280546106649061108a565b80156106af5780601f10610686576101008083540402835291602001916106af565b820191905f5260205f20905b81548152906001019060200180831161069257829003601f168201915b50505050509050919050565b5f6106c46108a2565b60078054905f6106d3836110c2565b9190505590506106e38382610b0f565b5f8181526008602052604090206106fa838261112a565b5092915050565b6001600160a01b039182165f90815260056020908152604080832093909416825291909152205460ff1690565b6107366108a2565b6001600160a01b03811661075f57604051631e4fbdf760e01b81525f60048201526024016103dc565b610768816108cf565b50565b5f818152600260205260408120546001600160a01b0316806102ec57604051637e27328960e01b8152600481018490526024016103dc565b61057b8383836001610b28565b5f828152600260205260408120546001600160a01b03908116908316156107dc576107dc818486610c2c565b6001600160a01b03811615610816576107f75f855f80610b28565b6001600160a01b0381165f90815260036020526040902080545f190190555b6001600160a01b03851615610844576001600160a01b0385165f908152600360205260409020805460010190555b5f8481526002602052604080822080546001600160a01b0319166001600160a01b0389811691821790925591518793918516917fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef91a4949350505050565b6006546001600160a01b031633146105e05760405163118cdaa760e01b81523360048201526024016103dc565b600680546001600160a01b038381166001600160a01b0319831681179093556040519116919082907f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0905f90a35050565b6001600160a01b0383166109495760405163a9fbf51f60e01b81525f60048201526024016103dc565b6001600160a01b03821661097b57604051630b61174360e31b81526001600160a01b03831660048201526024016103dc565b6001600160a01b038381165f81815260056020908152604080832094871680845294825291829020805460ff191686151590811790915591519182527f17307eab39ab6107e8899845ad3d59bd9653f200f220920489ca2b5937696c31910160405180910390a3505050565b6001600160a01b0383163b15610b0857604051630a85bd0160e11b81526001600160a01b0384169063150b7a0290610a299088908890879087906004016111e6565b6020604051808303815f875af1925050508015610a63575060408051601f3d908101601f19168201909252610a6091810190611222565b60015b610aca573d808015610a90576040519150601f19603f3d011682016040523d82523d5f602084013e610a95565b606091505b5080515f03610ac257604051633250574960e11b81526001600160a01b03851660048201526024016103dc565b805160208201fd5b6001600160e01b03198116630a85bd0160e11b14610b0657604051633250574960e11b81526001600160a01b03851660048201526024016103dc565b505b5050505050565b6103b3828260405180602001604052805f815250610c90565b8080610b3c57506001600160a01b03821615155b15610bfd575f610b4b8461076b565b90506001600160a01b03831615801590610b775750826001600160a01b0316816001600160a01b031614155b8015610b8a5750610b888184610701565b155b15610bb35760405163a9fbf51f60e01b81526001600160a01b03841660048201526024016103dc565b8115610bfb5783856001600160a01b0316826001600160a01b03167f8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b92560405160405180910390a45b505b50505f90815260046020526040902080546001600160a01b0319166001600160a01b0392909216919091179055565b610c37838383610ca7565b61057b576001600160a01b038316610c6557604051637e27328960e01b8152600481018290526024016103dc565b60405163177e802f60e01b81526001600160a01b0383166004820152602481018290526044016103dc565b610c9a8383610d0b565b61057b335f8585856109e7565b5f6001600160a01b03831615801590610d035750826001600160a01b0316846001600160a01b03161480610ce05750610ce08484610701565b80610d0357505f828152600460205260409020546001600160a01b038481169116145b949350505050565b6001600160a01b038216610d3457604051633250574960e11b81525f60048201526024016103dc565b5f610d4083835f6107b0565b90506001600160a01b0381161561057b576040516339e3563760e11b81525f60048201526024016103dc565b6001600160e01b031981168114610768575f80fd5b5f60208284031215610d91575f80fd5b8135610d9c81610d6c565b9392505050565b5f81518084525f5b81811015610dc757602081850181015186830182015201610dab565b505f602082860101526020601f19601f83011685010191505092915050565b602081525f610d9c6020830184610da3565b5f60208284031215610e08575f80fd5b5035919050565b80356001600160a01b0381168114610e25575f80fd5b919050565b5f8060408385031215610e3b575f80fd5b610e4483610e0f565b946020939093013593505050565b5f805f60608486031215610e64575f80fd5b610e6d84610e0f565b9250610e7b60208501610e0f565b9150604084013590509250925092565b5f8060408385031215610e9c575f80fd5b50508035926020909101359150565b5f60208284031215610ebb575f80fd5b610d9c82610e0f565b5f8060408385031215610ed5575f80fd5b610ede83610e0f565b915060208301358015158114610ef2575f80fd5b809150509250929050565b634e487b7160e01b5f52604160045260245ffd5b5f67ffffffffffffffff80841115610f2b57610f2b610efd565b604051601f8501601f19908116603f01168101908282118183101715610f5357610f53610efd565b81604052809350858152868686011115610f6b575f80fd5b858560208301375f602087830101525050509392505050565b5f805f8060808587031215610f97575f80fd5b610fa085610e0f565b9350610fae60208601610e0f565b925060408501359150606085013567ffffffffffffffff811115610fd0575f80fd5b8501601f81018713610fe0575f80fd5b610fef87823560208401610f11565b91505092959194509250565b5f806040838503121561100c575f80fd5b61101583610e0f565b9150602083013567ffffffffffffffff811115611030575f80fd5b8301601f81018513611040575f80fd5b61104f85823560208401610f11565b9150509250929050565b5f806040838503121561106a575f80fd5b61107383610e0f565b915061108160208401610e0f565b90509250929050565b600181811c9082168061109e57607f821691505b6020821081036110bc57634e487b7160e01b5f52602260045260245ffd5b50919050565b5f600182016110df57634e487b7160e01b5f52601160045260245ffd5b5060010190565b601f82111561057b57805f5260205f20601f840160051c8101602085101561110b5750805b601f840160051c820191505b81811015610b08575f8155600101611117565b815167ffffffffffffffff81111561114457611144610efd565b61115881611152845461108a565b846110e6565b602080601f83116001811461118b575f84156111745750858301515b5f19600386901b1c1916600185901b178555610b06565b5f85815260208120601f198616915b828110156111b95788860151825594840194600190910190840161119a565b50858210156111d657878501515f19600388901b60f8161c191681555b5050505050600190811b01905550565b6001600160a01b03858116825284166020820152604081018390526080606082018190525f9061121890830184610da3565b9695505050505050565b5f60208284031215611232575f80fd5b8151610d9c81610d6c56fea264697066735822122022e78376b4d275cc9107c808493659165a1fa60a2be0152e8fb16903abcc7a7b64736f6c63430008180033",
}

// EraPersonaINFTABI is the input ABI used to generate the binding from.
// Deprecated: Use EraPersonaINFTMetaData.ABI instead.
var EraPersonaINFTABI = EraPersonaINFTMetaData.ABI

// EraPersonaINFTBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use EraPersonaINFTMetaData.Bin instead.
var EraPersonaINFTBin = EraPersonaINFTMetaData.Bin

// DeployEraPersonaINFT deploys a new Ethereum contract, binding an instance of EraPersonaINFT to it.
func DeployEraPersonaINFT(auth *bind.TransactOpts, backend bind.ContractBackend, initialOwner common.Address) (common.Address, *types.Transaction, *EraPersonaINFT, error) {
	parsed, err := EraPersonaINFTMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(EraPersonaINFTBin), backend, initialOwner)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &EraPersonaINFT{EraPersonaINFTCaller: EraPersonaINFTCaller{contract: contract}, EraPersonaINFTTransactor: EraPersonaINFTTransactor{contract: contract}, EraPersonaINFTFilterer: EraPersonaINFTFilterer{contract: contract}}, nil
}

// EraPersonaINFT is an auto generated Go binding around an Ethereum contract.
type EraPersonaINFT struct {
	EraPersonaINFTCaller     // Read-only binding to the contract
	EraPersonaINFTTransactor // Write-only binding to the contract
	EraPersonaINFTFilterer   // Log filterer for contract events
}

// EraPersonaINFTCaller is an auto generated read-only Go binding around an Ethereum contract.
type EraPersonaINFTCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// EraPersonaINFTTransactor is an auto generated write-only Go binding around an Ethereum contract.
type EraPersonaINFTTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// EraPersonaINFTFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type EraPersonaINFTFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// EraPersonaINFTSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type EraPersonaINFTSession struct {
	Contract     *EraPersonaINFT   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// EraPersonaINFTCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type EraPersonaINFTCallerSession struct {
	Contract *EraPersonaINFTCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// EraPersonaINFTTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type EraPersonaINFTTransactorSession struct {
	Contract     *EraPersonaINFTTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// EraPersonaINFTRaw is an auto generated low-level Go binding around an Ethereum contract.
type EraPersonaINFTRaw struct {
	Contract *EraPersonaINFT // Generic contract binding to access the raw methods on
}

// EraPersonaINFTCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type EraPersonaINFTCallerRaw struct {
	Contract *EraPersonaINFTCaller // Generic read-only contract binding to access the raw methods on
}

// EraPersonaINFTTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type EraPersonaINFTTransactorRaw struct {
	Contract *EraPersonaINFTTransactor // Generic write-only contract binding to access the raw methods on
}

// NewEraPersonaINFT creates a new instance of EraPersonaINFT, bound to a specific deployed contract.
func NewEraPersonaINFT(address common.Address, backend bind.ContractBackend) (*EraPersonaINFT, error) {
	contract, err := bindEraPersonaINFT(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFT{EraPersonaINFTCaller: EraPersonaINFTCaller{contract: contract}, EraPersonaINFTTransactor: EraPersonaINFTTransactor{contract: contract}, EraPersonaINFTFilterer: EraPersonaINFTFilterer{contract: contract}}, nil
}

// NewEraPersonaINFTCaller creates a new read-only instance of EraPersonaINFT, bound to a specific deployed contract.
func NewEraPersonaINFTCaller(address common.Address, caller bind.ContractCaller) (*EraPersonaINFTCaller, error) {
	contract, err := bindEraPersonaINFT(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTCaller{contract: contract}, nil
}

// NewEraPersonaINFTTransactor creates a new write-only instance of EraPersonaINFT, bound to a specific deployed contract.
func NewEraPersonaINFTTransactor(address common.Address, transactor bind.ContractTransactor) (*EraPersonaINFTTransactor, error) {
	contract, err := bindEraPersonaINFT(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTTransactor{contract: contract}, nil
}

// NewEraPersonaINFTFilterer creates a new log filterer instance of EraPersonaINFT, bound to a specific deployed contract.
func NewEraPersonaINFTFilterer(address common.Address, filterer bind.ContractFilterer) (*EraPersonaINFTFilterer, error) {
	contract, err := bindEraPersonaINFT(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTFilterer{contract: contract}, nil
}

// bindEraPersonaINFT binds a generic wrapper to an already deployed contract.
func bindEraPersonaINFT(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := EraPersonaINFTMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_EraPersonaINFT *EraPersonaINFTRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _EraPersonaINFT.Contract.EraPersonaINFTCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_EraPersonaINFT *EraPersonaINFTRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.EraPersonaINFTTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_EraPersonaINFT *EraPersonaINFTRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.EraPersonaINFTTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_EraPersonaINFT *EraPersonaINFTCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _EraPersonaINFT.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_EraPersonaINFT *EraPersonaINFTTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_EraPersonaINFT *EraPersonaINFTTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.contract.Transact(opts, method, params...)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address owner) view returns(uint256)
func (_EraPersonaINFT *EraPersonaINFTCaller) BalanceOf(opts *bind.CallOpts, owner common.Address) (*big.Int, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "balanceOf", owner)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address owner) view returns(uint256)
func (_EraPersonaINFT *EraPersonaINFTSession) BalanceOf(owner common.Address) (*big.Int, error) {
	return _EraPersonaINFT.Contract.BalanceOf(&_EraPersonaINFT.CallOpts, owner)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address owner) view returns(uint256)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) BalanceOf(owner common.Address) (*big.Int, error) {
	return _EraPersonaINFT.Contract.BalanceOf(&_EraPersonaINFT.CallOpts, owner)
}

// GetApproved is a free data retrieval call binding the contract method 0x081812fc.
//
// Solidity: function getApproved(uint256 tokenId) view returns(address)
func (_EraPersonaINFT *EraPersonaINFTCaller) GetApproved(opts *bind.CallOpts, tokenId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "getApproved", tokenId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetApproved is a free data retrieval call binding the contract method 0x081812fc.
//
// Solidity: function getApproved(uint256 tokenId) view returns(address)
func (_EraPersonaINFT *EraPersonaINFTSession) GetApproved(tokenId *big.Int) (common.Address, error) {
	return _EraPersonaINFT.Contract.GetApproved(&_EraPersonaINFT.CallOpts, tokenId)
}

// GetApproved is a free data retrieval call binding the contract method 0x081812fc.
//
// Solidity: function getApproved(uint256 tokenId) view returns(address)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) GetApproved(tokenId *big.Int) (common.Address, error) {
	return _EraPersonaINFT.Contract.GetApproved(&_EraPersonaINFT.CallOpts, tokenId)
}

// IsApprovedForAll is a free data retrieval call binding the contract method 0xe985e9c5.
//
// Solidity: function isApprovedForAll(address owner, address operator) view returns(bool)
func (_EraPersonaINFT *EraPersonaINFTCaller) IsApprovedForAll(opts *bind.CallOpts, owner common.Address, operator common.Address) (bool, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "isApprovedForAll", owner, operator)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsApprovedForAll is a free data retrieval call binding the contract method 0xe985e9c5.
//
// Solidity: function isApprovedForAll(address owner, address operator) view returns(bool)
func (_EraPersonaINFT *EraPersonaINFTSession) IsApprovedForAll(owner common.Address, operator common.Address) (bool, error) {
	return _EraPersonaINFT.Contract.IsApprovedForAll(&_EraPersonaINFT.CallOpts, owner, operator)
}

// IsApprovedForAll is a free data retrieval call binding the contract method 0xe985e9c5.
//
// Solidity: function isApprovedForAll(address owner, address operator) view returns(bool)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) IsApprovedForAll(owner common.Address, operator common.Address) (bool, error) {
	return _EraPersonaINFT.Contract.IsApprovedForAll(&_EraPersonaINFT.CallOpts, owner, operator)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_EraPersonaINFT *EraPersonaINFTCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_EraPersonaINFT *EraPersonaINFTSession) Name() (string, error) {
	return _EraPersonaINFT.Contract.Name(&_EraPersonaINFT.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) Name() (string, error) {
	return _EraPersonaINFT.Contract.Name(&_EraPersonaINFT.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_EraPersonaINFT *EraPersonaINFTCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_EraPersonaINFT *EraPersonaINFTSession) Owner() (common.Address, error) {
	return _EraPersonaINFT.Contract.Owner(&_EraPersonaINFT.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) Owner() (common.Address, error) {
	return _EraPersonaINFT.Contract.Owner(&_EraPersonaINFT.CallOpts)
}

// OwnerOf is a free data retrieval call binding the contract method 0x6352211e.
//
// Solidity: function ownerOf(uint256 tokenId) view returns(address)
func (_EraPersonaINFT *EraPersonaINFTCaller) OwnerOf(opts *bind.CallOpts, tokenId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "ownerOf", tokenId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// OwnerOf is a free data retrieval call binding the contract method 0x6352211e.
//
// Solidity: function ownerOf(uint256 tokenId) view returns(address)
func (_EraPersonaINFT *EraPersonaINFTSession) OwnerOf(tokenId *big.Int) (common.Address, error) {
	return _EraPersonaINFT.Contract.OwnerOf(&_EraPersonaINFT.CallOpts, tokenId)
}

// OwnerOf is a free data retrieval call binding the contract method 0x6352211e.
//
// Solidity: function ownerOf(uint256 tokenId) view returns(address)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) OwnerOf(tokenId *big.Int) (common.Address, error) {
	return _EraPersonaINFT.Contract.OwnerOf(&_EraPersonaINFT.CallOpts, tokenId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_EraPersonaINFT *EraPersonaINFTCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_EraPersonaINFT *EraPersonaINFTSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _EraPersonaINFT.Contract.SupportsInterface(&_EraPersonaINFT.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _EraPersonaINFT.Contract.SupportsInterface(&_EraPersonaINFT.CallOpts, interfaceId)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_EraPersonaINFT *EraPersonaINFTCaller) Symbol(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "symbol")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_EraPersonaINFT *EraPersonaINFTSession) Symbol() (string, error) {
	return _EraPersonaINFT.Contract.Symbol(&_EraPersonaINFT.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) Symbol() (string, error) {
	return _EraPersonaINFT.Contract.Symbol(&_EraPersonaINFT.CallOpts)
}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_EraPersonaINFT *EraPersonaINFTCaller) TokenURI(opts *bind.CallOpts, tokenId *big.Int) (string, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "tokenURI", tokenId)

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_EraPersonaINFT *EraPersonaINFTSession) TokenURI(tokenId *big.Int) (string, error) {
	return _EraPersonaINFT.Contract.TokenURI(&_EraPersonaINFT.CallOpts, tokenId)
}

// TokenURI is a free data retrieval call binding the contract method 0xc87b56dd.
//
// Solidity: function tokenURI(uint256 tokenId) view returns(string)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) TokenURI(tokenId *big.Int) (string, error) {
	return _EraPersonaINFT.Contract.TokenURI(&_EraPersonaINFT.CallOpts, tokenId)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_EraPersonaINFT *EraPersonaINFTCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _EraPersonaINFT.contract.Call(opts, &out, "totalSupply")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_EraPersonaINFT *EraPersonaINFTSession) TotalSupply() (*big.Int, error) {
	return _EraPersonaINFT.Contract.TotalSupply(&_EraPersonaINFT.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_EraPersonaINFT *EraPersonaINFTCallerSession) TotalSupply() (*big.Int, error) {
	return _EraPersonaINFT.Contract.TotalSupply(&_EraPersonaINFT.CallOpts)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) Approve(opts *bind.TransactOpts, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "approve", to, tokenId)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTSession) Approve(to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.Approve(&_EraPersonaINFT.TransactOpts, to, tokenId)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) Approve(to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.Approve(&_EraPersonaINFT.TransactOpts, to, tokenId)
}

// Mint is a paid mutator transaction binding the contract method 0xd0def521.
//
// Solidity: function mint(address to, string uri) returns(uint256 tokenId)
func (_EraPersonaINFT *EraPersonaINFTTransactor) Mint(opts *bind.TransactOpts, to common.Address, uri string) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "mint", to, uri)
}

// Mint is a paid mutator transaction binding the contract method 0xd0def521.
//
// Solidity: function mint(address to, string uri) returns(uint256 tokenId)
func (_EraPersonaINFT *EraPersonaINFTSession) Mint(to common.Address, uri string) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.Mint(&_EraPersonaINFT.TransactOpts, to, uri)
}

// Mint is a paid mutator transaction binding the contract method 0xd0def521.
//
// Solidity: function mint(address to, string uri) returns(uint256 tokenId)
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) Mint(to common.Address, uri string) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.Mint(&_EraPersonaINFT.TransactOpts, to, uri)
}

// RecordInvocation is a paid mutator transaction binding the contract method 0x2f625088.
//
// Solidity: function recordInvocation(uint256 tokenId, bytes32 receiptHash) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) RecordInvocation(opts *bind.TransactOpts, tokenId *big.Int, receiptHash [32]byte) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "recordInvocation", tokenId, receiptHash)
}

// RecordInvocation is a paid mutator transaction binding the contract method 0x2f625088.
//
// Solidity: function recordInvocation(uint256 tokenId, bytes32 receiptHash) returns()
func (_EraPersonaINFT *EraPersonaINFTSession) RecordInvocation(tokenId *big.Int, receiptHash [32]byte) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.RecordInvocation(&_EraPersonaINFT.TransactOpts, tokenId, receiptHash)
}

// RecordInvocation is a paid mutator transaction binding the contract method 0x2f625088.
//
// Solidity: function recordInvocation(uint256 tokenId, bytes32 receiptHash) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) RecordInvocation(tokenId *big.Int, receiptHash [32]byte) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.RecordInvocation(&_EraPersonaINFT.TransactOpts, tokenId, receiptHash)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_EraPersonaINFT *EraPersonaINFTSession) RenounceOwnership() (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.RenounceOwnership(&_EraPersonaINFT.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.RenounceOwnership(&_EraPersonaINFT.TransactOpts)
}

// SafeTransferFrom is a paid mutator transaction binding the contract method 0x42842e0e.
//
// Solidity: function safeTransferFrom(address from, address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) SafeTransferFrom(opts *bind.TransactOpts, from common.Address, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "safeTransferFrom", from, to, tokenId)
}

// SafeTransferFrom is a paid mutator transaction binding the contract method 0x42842e0e.
//
// Solidity: function safeTransferFrom(address from, address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTSession) SafeTransferFrom(from common.Address, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.SafeTransferFrom(&_EraPersonaINFT.TransactOpts, from, to, tokenId)
}

// SafeTransferFrom is a paid mutator transaction binding the contract method 0x42842e0e.
//
// Solidity: function safeTransferFrom(address from, address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) SafeTransferFrom(from common.Address, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.SafeTransferFrom(&_EraPersonaINFT.TransactOpts, from, to, tokenId)
}

// SafeTransferFrom0 is a paid mutator transaction binding the contract method 0xb88d4fde.
//
// Solidity: function safeTransferFrom(address from, address to, uint256 tokenId, bytes data) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) SafeTransferFrom0(opts *bind.TransactOpts, from common.Address, to common.Address, tokenId *big.Int, data []byte) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "safeTransferFrom0", from, to, tokenId, data)
}

// SafeTransferFrom0 is a paid mutator transaction binding the contract method 0xb88d4fde.
//
// Solidity: function safeTransferFrom(address from, address to, uint256 tokenId, bytes data) returns()
func (_EraPersonaINFT *EraPersonaINFTSession) SafeTransferFrom0(from common.Address, to common.Address, tokenId *big.Int, data []byte) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.SafeTransferFrom0(&_EraPersonaINFT.TransactOpts, from, to, tokenId, data)
}

// SafeTransferFrom0 is a paid mutator transaction binding the contract method 0xb88d4fde.
//
// Solidity: function safeTransferFrom(address from, address to, uint256 tokenId, bytes data) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) SafeTransferFrom0(from common.Address, to common.Address, tokenId *big.Int, data []byte) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.SafeTransferFrom0(&_EraPersonaINFT.TransactOpts, from, to, tokenId, data)
}

// SetApprovalForAll is a paid mutator transaction binding the contract method 0xa22cb465.
//
// Solidity: function setApprovalForAll(address operator, bool approved) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) SetApprovalForAll(opts *bind.TransactOpts, operator common.Address, approved bool) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "setApprovalForAll", operator, approved)
}

// SetApprovalForAll is a paid mutator transaction binding the contract method 0xa22cb465.
//
// Solidity: function setApprovalForAll(address operator, bool approved) returns()
func (_EraPersonaINFT *EraPersonaINFTSession) SetApprovalForAll(operator common.Address, approved bool) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.SetApprovalForAll(&_EraPersonaINFT.TransactOpts, operator, approved)
}

// SetApprovalForAll is a paid mutator transaction binding the contract method 0xa22cb465.
//
// Solidity: function setApprovalForAll(address operator, bool approved) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) SetApprovalForAll(operator common.Address, approved bool) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.SetApprovalForAll(&_EraPersonaINFT.TransactOpts, operator, approved)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) TransferFrom(opts *bind.TransactOpts, from common.Address, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "transferFrom", from, to, tokenId)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTSession) TransferFrom(from common.Address, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.TransferFrom(&_EraPersonaINFT.TransactOpts, from, to, tokenId)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 tokenId) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) TransferFrom(from common.Address, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.TransferFrom(&_EraPersonaINFT.TransactOpts, from, to, tokenId)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _EraPersonaINFT.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_EraPersonaINFT *EraPersonaINFTSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.TransferOwnership(&_EraPersonaINFT.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_EraPersonaINFT *EraPersonaINFTTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _EraPersonaINFT.Contract.TransferOwnership(&_EraPersonaINFT.TransactOpts, newOwner)
}

// EraPersonaINFTApprovalIterator is returned from FilterApproval and is used to iterate over the raw logs and unpacked data for Approval events raised by the EraPersonaINFT contract.
type EraPersonaINFTApprovalIterator struct {
	Event *EraPersonaINFTApproval // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *EraPersonaINFTApprovalIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(EraPersonaINFTApproval)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(EraPersonaINFTApproval)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *EraPersonaINFTApprovalIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *EraPersonaINFTApprovalIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// EraPersonaINFTApproval represents a Approval event raised by the EraPersonaINFT contract.
type EraPersonaINFTApproval struct {
	Owner    common.Address
	Approved common.Address
	TokenId  *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterApproval is a free log retrieval operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed approved, uint256 indexed tokenId)
func (_EraPersonaINFT *EraPersonaINFTFilterer) FilterApproval(opts *bind.FilterOpts, owner []common.Address, approved []common.Address, tokenId []*big.Int) (*EraPersonaINFTApprovalIterator, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var approvedRule []interface{}
	for _, approvedItem := range approved {
		approvedRule = append(approvedRule, approvedItem)
	}
	var tokenIdRule []interface{}
	for _, tokenIdItem := range tokenId {
		tokenIdRule = append(tokenIdRule, tokenIdItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.FilterLogs(opts, "Approval", ownerRule, approvedRule, tokenIdRule)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTApprovalIterator{contract: _EraPersonaINFT.contract, event: "Approval", logs: logs, sub: sub}, nil
}

// WatchApproval is a free log subscription operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed approved, uint256 indexed tokenId)
func (_EraPersonaINFT *EraPersonaINFTFilterer) WatchApproval(opts *bind.WatchOpts, sink chan<- *EraPersonaINFTApproval, owner []common.Address, approved []common.Address, tokenId []*big.Int) (event.Subscription, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var approvedRule []interface{}
	for _, approvedItem := range approved {
		approvedRule = append(approvedRule, approvedItem)
	}
	var tokenIdRule []interface{}
	for _, tokenIdItem := range tokenId {
		tokenIdRule = append(tokenIdRule, tokenIdItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.WatchLogs(opts, "Approval", ownerRule, approvedRule, tokenIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(EraPersonaINFTApproval)
				if err := _EraPersonaINFT.contract.UnpackLog(event, "Approval", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseApproval is a log parse operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed approved, uint256 indexed tokenId)
func (_EraPersonaINFT *EraPersonaINFTFilterer) ParseApproval(log types.Log) (*EraPersonaINFTApproval, error) {
	event := new(EraPersonaINFTApproval)
	if err := _EraPersonaINFT.contract.UnpackLog(event, "Approval", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// EraPersonaINFTApprovalForAllIterator is returned from FilterApprovalForAll and is used to iterate over the raw logs and unpacked data for ApprovalForAll events raised by the EraPersonaINFT contract.
type EraPersonaINFTApprovalForAllIterator struct {
	Event *EraPersonaINFTApprovalForAll // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *EraPersonaINFTApprovalForAllIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(EraPersonaINFTApprovalForAll)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(EraPersonaINFTApprovalForAll)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *EraPersonaINFTApprovalForAllIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *EraPersonaINFTApprovalForAllIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// EraPersonaINFTApprovalForAll represents a ApprovalForAll event raised by the EraPersonaINFT contract.
type EraPersonaINFTApprovalForAll struct {
	Owner    common.Address
	Operator common.Address
	Approved bool
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterApprovalForAll is a free log retrieval operation binding the contract event 0x17307eab39ab6107e8899845ad3d59bd9653f200f220920489ca2b5937696c31.
//
// Solidity: event ApprovalForAll(address indexed owner, address indexed operator, bool approved)
func (_EraPersonaINFT *EraPersonaINFTFilterer) FilterApprovalForAll(opts *bind.FilterOpts, owner []common.Address, operator []common.Address) (*EraPersonaINFTApprovalForAllIterator, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var operatorRule []interface{}
	for _, operatorItem := range operator {
		operatorRule = append(operatorRule, operatorItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.FilterLogs(opts, "ApprovalForAll", ownerRule, operatorRule)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTApprovalForAllIterator{contract: _EraPersonaINFT.contract, event: "ApprovalForAll", logs: logs, sub: sub}, nil
}

// WatchApprovalForAll is a free log subscription operation binding the contract event 0x17307eab39ab6107e8899845ad3d59bd9653f200f220920489ca2b5937696c31.
//
// Solidity: event ApprovalForAll(address indexed owner, address indexed operator, bool approved)
func (_EraPersonaINFT *EraPersonaINFTFilterer) WatchApprovalForAll(opts *bind.WatchOpts, sink chan<- *EraPersonaINFTApprovalForAll, owner []common.Address, operator []common.Address) (event.Subscription, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var operatorRule []interface{}
	for _, operatorItem := range operator {
		operatorRule = append(operatorRule, operatorItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.WatchLogs(opts, "ApprovalForAll", ownerRule, operatorRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(EraPersonaINFTApprovalForAll)
				if err := _EraPersonaINFT.contract.UnpackLog(event, "ApprovalForAll", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseApprovalForAll is a log parse operation binding the contract event 0x17307eab39ab6107e8899845ad3d59bd9653f200f220920489ca2b5937696c31.
//
// Solidity: event ApprovalForAll(address indexed owner, address indexed operator, bool approved)
func (_EraPersonaINFT *EraPersonaINFTFilterer) ParseApprovalForAll(log types.Log) (*EraPersonaINFTApprovalForAll, error) {
	event := new(EraPersonaINFTApprovalForAll)
	if err := _EraPersonaINFT.contract.UnpackLog(event, "ApprovalForAll", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// EraPersonaINFTInvocationIterator is returned from FilterInvocation and is used to iterate over the raw logs and unpacked data for Invocation events raised by the EraPersonaINFT contract.
type EraPersonaINFTInvocationIterator struct {
	Event *EraPersonaINFTInvocation // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *EraPersonaINFTInvocationIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(EraPersonaINFTInvocation)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(EraPersonaINFTInvocation)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *EraPersonaINFTInvocationIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *EraPersonaINFTInvocationIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// EraPersonaINFTInvocation represents a Invocation event raised by the EraPersonaINFT contract.
type EraPersonaINFTInvocation struct {
	TokenId     *big.Int
	ReceiptHash [32]byte
	Ts          *big.Int
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterInvocation is a free log retrieval operation binding the contract event 0x5299a77d2b4293488895b6dea81d075c8c010c488358ccd3c58c0ada2070eac8.
//
// Solidity: event Invocation(uint256 indexed tokenId, bytes32 indexed receiptHash, uint256 indexed ts)
func (_EraPersonaINFT *EraPersonaINFTFilterer) FilterInvocation(opts *bind.FilterOpts, tokenId []*big.Int, receiptHash [][32]byte, ts []*big.Int) (*EraPersonaINFTInvocationIterator, error) {

	var tokenIdRule []interface{}
	for _, tokenIdItem := range tokenId {
		tokenIdRule = append(tokenIdRule, tokenIdItem)
	}
	var receiptHashRule []interface{}
	for _, receiptHashItem := range receiptHash {
		receiptHashRule = append(receiptHashRule, receiptHashItem)
	}
	var tsRule []interface{}
	for _, tsItem := range ts {
		tsRule = append(tsRule, tsItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.FilterLogs(opts, "Invocation", tokenIdRule, receiptHashRule, tsRule)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTInvocationIterator{contract: _EraPersonaINFT.contract, event: "Invocation", logs: logs, sub: sub}, nil
}

// WatchInvocation is a free log subscription operation binding the contract event 0x5299a77d2b4293488895b6dea81d075c8c010c488358ccd3c58c0ada2070eac8.
//
// Solidity: event Invocation(uint256 indexed tokenId, bytes32 indexed receiptHash, uint256 indexed ts)
func (_EraPersonaINFT *EraPersonaINFTFilterer) WatchInvocation(opts *bind.WatchOpts, sink chan<- *EraPersonaINFTInvocation, tokenId []*big.Int, receiptHash [][32]byte, ts []*big.Int) (event.Subscription, error) {

	var tokenIdRule []interface{}
	for _, tokenIdItem := range tokenId {
		tokenIdRule = append(tokenIdRule, tokenIdItem)
	}
	var receiptHashRule []interface{}
	for _, receiptHashItem := range receiptHash {
		receiptHashRule = append(receiptHashRule, receiptHashItem)
	}
	var tsRule []interface{}
	for _, tsItem := range ts {
		tsRule = append(tsRule, tsItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.WatchLogs(opts, "Invocation", tokenIdRule, receiptHashRule, tsRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(EraPersonaINFTInvocation)
				if err := _EraPersonaINFT.contract.UnpackLog(event, "Invocation", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInvocation is a log parse operation binding the contract event 0x5299a77d2b4293488895b6dea81d075c8c010c488358ccd3c58c0ada2070eac8.
//
// Solidity: event Invocation(uint256 indexed tokenId, bytes32 indexed receiptHash, uint256 indexed ts)
func (_EraPersonaINFT *EraPersonaINFTFilterer) ParseInvocation(log types.Log) (*EraPersonaINFTInvocation, error) {
	event := new(EraPersonaINFTInvocation)
	if err := _EraPersonaINFT.contract.UnpackLog(event, "Invocation", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// EraPersonaINFTOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the EraPersonaINFT contract.
type EraPersonaINFTOwnershipTransferredIterator struct {
	Event *EraPersonaINFTOwnershipTransferred // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *EraPersonaINFTOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(EraPersonaINFTOwnershipTransferred)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(EraPersonaINFTOwnershipTransferred)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *EraPersonaINFTOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *EraPersonaINFTOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// EraPersonaINFTOwnershipTransferred represents a OwnershipTransferred event raised by the EraPersonaINFT contract.
type EraPersonaINFTOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_EraPersonaINFT *EraPersonaINFTFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*EraPersonaINFTOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTOwnershipTransferredIterator{contract: _EraPersonaINFT.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_EraPersonaINFT *EraPersonaINFTFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *EraPersonaINFTOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(EraPersonaINFTOwnershipTransferred)
				if err := _EraPersonaINFT.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_EraPersonaINFT *EraPersonaINFTFilterer) ParseOwnershipTransferred(log types.Log) (*EraPersonaINFTOwnershipTransferred, error) {
	event := new(EraPersonaINFTOwnershipTransferred)
	if err := _EraPersonaINFT.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// EraPersonaINFTTransferIterator is returned from FilterTransfer and is used to iterate over the raw logs and unpacked data for Transfer events raised by the EraPersonaINFT contract.
type EraPersonaINFTTransferIterator struct {
	Event *EraPersonaINFTTransfer // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *EraPersonaINFTTransferIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(EraPersonaINFTTransfer)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(EraPersonaINFTTransfer)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *EraPersonaINFTTransferIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *EraPersonaINFTTransferIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// EraPersonaINFTTransfer represents a Transfer event raised by the EraPersonaINFT contract.
type EraPersonaINFTTransfer struct {
	From    common.Address
	To      common.Address
	TokenId *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterTransfer is a free log retrieval operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 indexed tokenId)
func (_EraPersonaINFT *EraPersonaINFTFilterer) FilterTransfer(opts *bind.FilterOpts, from []common.Address, to []common.Address, tokenId []*big.Int) (*EraPersonaINFTTransferIterator, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}
	var tokenIdRule []interface{}
	for _, tokenIdItem := range tokenId {
		tokenIdRule = append(tokenIdRule, tokenIdItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.FilterLogs(opts, "Transfer", fromRule, toRule, tokenIdRule)
	if err != nil {
		return nil, err
	}
	return &EraPersonaINFTTransferIterator{contract: _EraPersonaINFT.contract, event: "Transfer", logs: logs, sub: sub}, nil
}

// WatchTransfer is a free log subscription operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 indexed tokenId)
func (_EraPersonaINFT *EraPersonaINFTFilterer) WatchTransfer(opts *bind.WatchOpts, sink chan<- *EraPersonaINFTTransfer, from []common.Address, to []common.Address, tokenId []*big.Int) (event.Subscription, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}
	var tokenIdRule []interface{}
	for _, tokenIdItem := range tokenId {
		tokenIdRule = append(tokenIdRule, tokenIdItem)
	}

	logs, sub, err := _EraPersonaINFT.contract.WatchLogs(opts, "Transfer", fromRule, toRule, tokenIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(EraPersonaINFTTransfer)
				if err := _EraPersonaINFT.contract.UnpackLog(event, "Transfer", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseTransfer is a log parse operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 indexed tokenId)
func (_EraPersonaINFT *EraPersonaINFTFilterer) ParseTransfer(log types.Log) (*EraPersonaINFTTransfer, error) {
	event := new(EraPersonaINFTTransfer)
	if err := _EraPersonaINFT.contract.UnpackLog(event, "Transfer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
