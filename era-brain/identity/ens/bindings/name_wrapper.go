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

// NameWrapperMetaData contains all meta data concerning the NameWrapper contract.
var NameWrapperMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"ownerOf\",\"inputs\":[{\"name\":\"tokenId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"setSubnodeRecord\",\"inputs\":[{\"name\":\"parentNode\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"label\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[{\"name\":\"node\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"testMint\",\"inputs\":[{\"name\":\"parentNode\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"}]",
	Bin: "0x608060405234801561000f575f80fd5b506103678061001d5f395ff3fe608060405234801561000f575f80fd5b506004361061003f575f3560e01c806324c1af44146100435780636352211e14610069578063bb0f273d146100a9575b5f80fd5b61005661005136600461021b565b6100e6565b6040519081526020015b60405180910390f35b6100916100773660046102e1565b5f908152602081905260409020546001600160a01b031690565b6040516001600160a01b039091168152602001610060565b6100e46100b73660046102f8565b5f9182526020829052604090912080546001600160a01b0319166001600160a01b03909216919091179055565b005b5f888152602081905260408120546001600160a01b0316331461015b5760405162461bcd60e51b8152602060048201526024808201527f4d6f636b4e616d65577261707065723a206e6f74206f776e6572206f662070616044820152631c995b9d60e21b606482015260840160405180910390fd5b88888860405161016c929190610322565b60405190819003812061018b9291602001918252602082015260400190565b60408051601f1981840301815291815281516020928301205f81815292839052912080546001600160a01b0389166001600160a01b0319909116179055905098975050505050505050565b80356001600160a01b03811681146101ec575f80fd5b919050565b803567ffffffffffffffff811681146101ec575f80fd5b803563ffffffff811681146101ec575f80fd5b5f805f805f805f8060e0898b031215610232575f80fd5b88359750602089013567ffffffffffffffff80821115610250575f80fd5b818b0191508b601f830112610263575f80fd5b813581811115610271575f80fd5b8c6020828501011115610282575f80fd5b60208301995080985050505061029a60408a016101d6565b94506102a860608a016101d6565b93506102b660808a016101f1565b92506102c460a08a01610208565b91506102d260c08a016101f1565b90509295985092959890939650565b5f602082840312156102f1575f80fd5b5035919050565b5f8060408385031215610309575f80fd5b82359150610319602084016101d6565b90509250929050565b818382375f910190815291905056fea2646970667358221220f5ff81223edcd134df561bd34a08124def6b33899743d3e1b275ed870cf1274664736f6c63430008180033",
}

// NameWrapperABI is the input ABI used to generate the binding from.
// Deprecated: Use NameWrapperMetaData.ABI instead.
var NameWrapperABI = NameWrapperMetaData.ABI

// NameWrapperBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use NameWrapperMetaData.Bin instead.
var NameWrapperBin = NameWrapperMetaData.Bin

// DeployNameWrapper deploys a new Ethereum contract, binding an instance of NameWrapper to it.
func DeployNameWrapper(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *NameWrapper, error) {
	parsed, err := NameWrapperMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(NameWrapperBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &NameWrapper{NameWrapperCaller: NameWrapperCaller{contract: contract}, NameWrapperTransactor: NameWrapperTransactor{contract: contract}, NameWrapperFilterer: NameWrapperFilterer{contract: contract}}, nil
}

// NameWrapper is an auto generated Go binding around an Ethereum contract.
type NameWrapper struct {
	NameWrapperCaller     // Read-only binding to the contract
	NameWrapperTransactor // Write-only binding to the contract
	NameWrapperFilterer   // Log filterer for contract events
}

// NameWrapperCaller is an auto generated read-only Go binding around an Ethereum contract.
type NameWrapperCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NameWrapperTransactor is an auto generated write-only Go binding around an Ethereum contract.
type NameWrapperTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NameWrapperFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type NameWrapperFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NameWrapperSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type NameWrapperSession struct {
	Contract     *NameWrapper      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// NameWrapperCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type NameWrapperCallerSession struct {
	Contract *NameWrapperCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// NameWrapperTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type NameWrapperTransactorSession struct {
	Contract     *NameWrapperTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// NameWrapperRaw is an auto generated low-level Go binding around an Ethereum contract.
type NameWrapperRaw struct {
	Contract *NameWrapper // Generic contract binding to access the raw methods on
}

// NameWrapperCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type NameWrapperCallerRaw struct {
	Contract *NameWrapperCaller // Generic read-only contract binding to access the raw methods on
}

// NameWrapperTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type NameWrapperTransactorRaw struct {
	Contract *NameWrapperTransactor // Generic write-only contract binding to access the raw methods on
}

// NewNameWrapper creates a new instance of NameWrapper, bound to a specific deployed contract.
func NewNameWrapper(address common.Address, backend bind.ContractBackend) (*NameWrapper, error) {
	contract, err := bindNameWrapper(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &NameWrapper{NameWrapperCaller: NameWrapperCaller{contract: contract}, NameWrapperTransactor: NameWrapperTransactor{contract: contract}, NameWrapperFilterer: NameWrapperFilterer{contract: contract}}, nil
}

// NewNameWrapperCaller creates a new read-only instance of NameWrapper, bound to a specific deployed contract.
func NewNameWrapperCaller(address common.Address, caller bind.ContractCaller) (*NameWrapperCaller, error) {
	contract, err := bindNameWrapper(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &NameWrapperCaller{contract: contract}, nil
}

// NewNameWrapperTransactor creates a new write-only instance of NameWrapper, bound to a specific deployed contract.
func NewNameWrapperTransactor(address common.Address, transactor bind.ContractTransactor) (*NameWrapperTransactor, error) {
	contract, err := bindNameWrapper(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &NameWrapperTransactor{contract: contract}, nil
}

// NewNameWrapperFilterer creates a new log filterer instance of NameWrapper, bound to a specific deployed contract.
func NewNameWrapperFilterer(address common.Address, filterer bind.ContractFilterer) (*NameWrapperFilterer, error) {
	contract, err := bindNameWrapper(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &NameWrapperFilterer{contract: contract}, nil
}

// bindNameWrapper binds a generic wrapper to an already deployed contract.
func bindNameWrapper(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := NameWrapperMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_NameWrapper *NameWrapperRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _NameWrapper.Contract.NameWrapperCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_NameWrapper *NameWrapperRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _NameWrapper.Contract.NameWrapperTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_NameWrapper *NameWrapperRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _NameWrapper.Contract.NameWrapperTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_NameWrapper *NameWrapperCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _NameWrapper.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_NameWrapper *NameWrapperTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _NameWrapper.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_NameWrapper *NameWrapperTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _NameWrapper.Contract.contract.Transact(opts, method, params...)
}

// OwnerOf is a free data retrieval call binding the contract method 0x6352211e.
//
// Solidity: function ownerOf(uint256 tokenId) view returns(address)
func (_NameWrapper *NameWrapperCaller) OwnerOf(opts *bind.CallOpts, tokenId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _NameWrapper.contract.Call(opts, &out, "ownerOf", tokenId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// OwnerOf is a free data retrieval call binding the contract method 0x6352211e.
//
// Solidity: function ownerOf(uint256 tokenId) view returns(address)
func (_NameWrapper *NameWrapperSession) OwnerOf(tokenId *big.Int) (common.Address, error) {
	return _NameWrapper.Contract.OwnerOf(&_NameWrapper.CallOpts, tokenId)
}

// OwnerOf is a free data retrieval call binding the contract method 0x6352211e.
//
// Solidity: function ownerOf(uint256 tokenId) view returns(address)
func (_NameWrapper *NameWrapperCallerSession) OwnerOf(tokenId *big.Int) (common.Address, error) {
	return _NameWrapper.Contract.OwnerOf(&_NameWrapper.CallOpts, tokenId)
}

// SetSubnodeRecord is a paid mutator transaction binding the contract method 0x24c1af44.
//
// Solidity: function setSubnodeRecord(bytes32 parentNode, string label, address owner, address , uint64 , uint32 , uint64 ) returns(bytes32 node)
func (_NameWrapper *NameWrapperTransactor) SetSubnodeRecord(opts *bind.TransactOpts, parentNode [32]byte, label string, owner common.Address, arg3 common.Address, arg4 uint64, arg5 uint32, arg6 uint64) (*types.Transaction, error) {
	return _NameWrapper.contract.Transact(opts, "setSubnodeRecord", parentNode, label, owner, arg3, arg4, arg5, arg6)
}

// SetSubnodeRecord is a paid mutator transaction binding the contract method 0x24c1af44.
//
// Solidity: function setSubnodeRecord(bytes32 parentNode, string label, address owner, address , uint64 , uint32 , uint64 ) returns(bytes32 node)
func (_NameWrapper *NameWrapperSession) SetSubnodeRecord(parentNode [32]byte, label string, owner common.Address, arg3 common.Address, arg4 uint64, arg5 uint32, arg6 uint64) (*types.Transaction, error) {
	return _NameWrapper.Contract.SetSubnodeRecord(&_NameWrapper.TransactOpts, parentNode, label, owner, arg3, arg4, arg5, arg6)
}

// SetSubnodeRecord is a paid mutator transaction binding the contract method 0x24c1af44.
//
// Solidity: function setSubnodeRecord(bytes32 parentNode, string label, address owner, address , uint64 , uint32 , uint64 ) returns(bytes32 node)
func (_NameWrapper *NameWrapperTransactorSession) SetSubnodeRecord(parentNode [32]byte, label string, owner common.Address, arg3 common.Address, arg4 uint64, arg5 uint32, arg6 uint64) (*types.Transaction, error) {
	return _NameWrapper.Contract.SetSubnodeRecord(&_NameWrapper.TransactOpts, parentNode, label, owner, arg3, arg4, arg5, arg6)
}

// TestMint is a paid mutator transaction binding the contract method 0xbb0f273d.
//
// Solidity: function testMint(bytes32 parentNode, address to) returns()
func (_NameWrapper *NameWrapperTransactor) TestMint(opts *bind.TransactOpts, parentNode [32]byte, to common.Address) (*types.Transaction, error) {
	return _NameWrapper.contract.Transact(opts, "testMint", parentNode, to)
}

// TestMint is a paid mutator transaction binding the contract method 0xbb0f273d.
//
// Solidity: function testMint(bytes32 parentNode, address to) returns()
func (_NameWrapper *NameWrapperSession) TestMint(parentNode [32]byte, to common.Address) (*types.Transaction, error) {
	return _NameWrapper.Contract.TestMint(&_NameWrapper.TransactOpts, parentNode, to)
}

// TestMint is a paid mutator transaction binding the contract method 0xbb0f273d.
//
// Solidity: function testMint(bytes32 parentNode, address to) returns()
func (_NameWrapper *NameWrapperTransactorSession) TestMint(parentNode [32]byte, to common.Address) (*types.Transaction, error) {
	return _NameWrapper.Contract.TestMint(&_NameWrapper.TransactOpts, parentNode, to)
}
