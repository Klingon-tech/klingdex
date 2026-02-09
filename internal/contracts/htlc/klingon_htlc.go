// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package htlc

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

// KlingonHTLCSwap is an auto generated low-level Go binding around an user-defined struct.
type KlingonHTLCSwap struct {
	Sender     common.Address
	Receiver   common.Address
	Token      common.Address
	Amount     *big.Int
	DaoFee     *big.Int
	SecretHash [32]byte
	Timelock   *big.Int
	State      uint8
}

// KlingonHTLCMetaData contains all meta data concerning the KlingonHTLC contract.
var KlingonHTLCMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"_daoAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"FEE_DENOMINATOR\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MAX_FEE_BPS\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MAX_TIMELOCK\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MIN_SWAP_AMOUNT\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MIN_TIMELOCK\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"canClaim\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"canRefund\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"claim\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"secret\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"computeSwapId\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"secretHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"timelock\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"createSwapERC20\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"secretHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"timelock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"createSwapNative\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"secretHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"timelock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"daoAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"feeBps\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getChainId\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getSwap\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structKlingonHTLC.Swap\",\"components\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"daoFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"secretHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"timelock\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"state\",\"type\":\"uint8\",\"internalType\":\"enumKlingonHTLC.SwapState\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"paused\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"refund\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setDaoAddress\",\"inputs\":[{\"name\":\"_daoAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setFeeBps\",\"inputs\":[{\"name\":\"_feeBps\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setPaused\",\"inputs\":[{\"name\":\"_paused\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"swaps\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"daoFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"secretHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"timelock\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"state\",\"type\":\"uint8\",\"internalType\":\"enumKlingonHTLC.SwapState\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"timeUntilRefund\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"verifySecret\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"secret\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"DaoAddressUpdated\",\"inputs\":[{\"name\":\"oldDao\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newDao\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"FeeBpsUpdated\",\"inputs\":[{\"name\":\"oldFee\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"newFee\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Paused\",\"inputs\":[{\"name\":\"isPaused\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SwapClaimed\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"secret\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SwapCreated\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"sender\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"token\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"daoFee\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"secretHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"timelock\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"SwapRefunded\",\"inputs\":[{\"name\":\"swapId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"sender\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ContractPaused\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"FeeTooHigh\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAmount\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidDaoAddress\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidReceiver\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidSecret\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidTimelock\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotReceiver\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotSender\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"SwapAlreadyExists\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SwapNotActive\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TimelockNotExpired\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TransferFailed\",\"inputs\":[]}]",
	Bin: "0x6080604052601460035534801562000015575f80fd5b50604051620019a7380380620019a783398101604081905262000038916200012b565b60017f9b779b17422d0df92223018b32b4d1fa46e071723d6817e2486d003becc55f005533806200008257604051631e4fbdf760e01b81525f600482015260240160405180910390fd5b6200008d81620000dc565b506001600160a01b038116620000b65760405163193fd72360e11b815260040160405180910390fd5b600280546001600160a01b0319166001600160a01b03929092169190911790556200015a565b5f80546001600160a01b038381166001600160a01b0319831681178455604051919092169283917f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e09190a35050565b5f602082840312156200013c575f80fd5b81516001600160a01b038116811462000153575f80fd5b9392505050565b61183f80620001685f395ff3fe60806040526004361061017b575f3560e01c806389cdebe3116100cd5780639a3cac6a11610087578063eaaa98fa11610062578063eaaa98fa1461048a578063eb84e7f2146104a9578063f2fde38b1461052d578063fb0c75491461054c575f80fd5b80639a3cac6a14610441578063d55be8c614610460578063d73792a914610475575f80fd5b806389cdebe31461039d5780638c8c2003146103bc5780638da5cb5b146103db57806394f1825b146103f757806395824c681461040d57806399f34c121461042c575f80fd5b80633da0e66e11610138578063715018a611610113578063715018a61461032c5780637249fbb61461034057806372c27b621461035f57806384cc9dfb1461037e575f80fd5b80633da0e66e146102d457806344248c2c146103005780635c975abb14610313575f80fd5b806311758b141461017f57806316c38b3c146101b35780632131c68c146101d457806324a9d8531461020b5780632554c6e31461022e5780633408e470146102c2575b5f80fd5b34801561018a575f80fd5b5061019e6101993660046114c3565b610561565b60405190151581526020015b60405180910390f35b3480156101be575f80fd5b506101d26101cd3660046114e3565b6105e4565b005b3480156101df575f80fd5b506002546101f3906001600160a01b031681565b6040516001600160a01b0390911681526020016101aa565b348015610216575f80fd5b5061022060035481565b6040519081526020016101aa565b348015610239575f80fd5b5061022061024836600461151d565b6040516bffffffffffffffffffffffff19606089811b8216602084015288811b8216603484015287901b166048820152605c8101859052607c8101849052609c81018390524660bc82015260dc81018290525f9060fc01604051602081830303815290604052805190602001209050979650505050505050565b3480156102cd575f80fd5b5046610220565b3480156102df575f80fd5b506102f36102ee366004611580565b610633565b6040516101aa91906115cb565b6101d261030e36600461163a565b61071e565b34801561031e575f80fd5b5060045461019e9060ff1681565b348015610337575f80fd5b506101d2610906565b34801561034b575f80fd5b506101d261035a366004611580565b610919565b34801561036a575f80fd5b506101d2610379366004611580565b610abd565b348015610389575f80fd5b506101d26103983660046114c3565b610b29565b3480156103a8575f80fd5b506102206103b7366004611580565b610e0d565b3480156103c7575f80fd5b5061019e6103d6366004611580565b610e6b565b3480156103e6575f80fd5b505f546001600160a01b03166101f3565b348015610402575f80fd5b5061022062278d0081565b348015610418575f80fd5b506101d2610427366004611672565b610ea9565b348015610437575f80fd5b506102206103e881565b34801561044c575f80fd5b506101d261045b3660046116c6565b6110d3565b34801561046b575f80fd5b506102206101f481565b348015610480575f80fd5b5061022061271081565b348015610495575f80fd5b5061019e6104a4366004611580565b61115d565b3480156104b4575f80fd5b506105196104c3366004611580565b600160208190525f9182526040909120805491810154600282015460038301546004840154600585015460068601546007909601546001600160a01b039788169795861696959094169492939192909160ff1688565b6040516101aa9897969594939291906116df565b348015610538575f80fd5b506101d26105473660046116c6565b61119a565b348015610557575f80fd5b50610220610e1081565b5f828152600160209081526040808320600501548151928301859052916002910160408051601f198184030181529082905261059c91611736565b602060405180830381855afa1580156105b7573d5f803e3d5ffd5b5050506040513d601f19601f820116820180604052508101906105da9190611762565b1490505b92915050565b6105ec6111d9565b6004805460ff19168215159081179091556040519081527f0e2fb031ee032dc02d8011dc50b816eb450cf856abd8261680dac74f72165bd29060200160405180910390a150565b61067760408051610100810182525f80825260208201819052918101829052606081018290526080810182905260a0810182905260c081018290529060e082015290565b5f8281526001602081815260409283902083516101008101855281546001600160a01b03908116825293820154841692810192909252600281015490921692810192909252600380820154606084015260048201546080840152600582015460a0840152600682015460c0840152600782015460e084019160ff9091169081111561070457610704611597565b600381111561071557610715611597565b90525092915050565b610726611205565b60045460ff161561074a5760405163ab35696f60e01b815260040160405180910390fd5b61075684843484611220565b5f61271060035434610768919061178d565b61077291906117a4565b9050604051806101000160405280336001600160a01b03168152602001856001600160a01b031681526020015f6001600160a01b03168152602001348152602001828152602001848152602001838152602001600160038111156107d8576107d8611597565b90525f86815260016020818152604092839020845181546001600160a01b03199081166001600160a01b03928316178355928601518285018054851691831691909117905593850151600282018054909316941693909317905560608301516003808401919091556080840151600484015560a0840151600584015560c0840151600684015560e08401516007840180549193909260ff1990921691849081111561088557610885611597565b021790555050604080515f815234602082015290810183905260608101859052608081018490526001600160a01b0386169150339087907fe584d5707a073b24a82c4414bac2dd9c326e3a1eaeb0b74ca6c885c4a20de0fc9060a00160405180910390a45061090060015f805160206117ea83398151915255565b50505050565b61090e6111d9565b6109175f611306565b565b610921611205565b5f81815260016020819052604090912090600782015460ff16600381111561094b5761094b611597565b146109695760405163cac0f49d60e01b815260040160405180910390fd5b80546001600160a01b031633146109935760405163b2c3aa6b60e01b815260040160405180910390fd5b80600601544210156109b85760405163621e25c360e01b815260040160405180910390fd5b60078101805460ff1916600317905560028101546001600160a01b0316610a5357805460038201546040515f926001600160a01b031691908381818185875af1925050503d805f8114610a26576040519150601f19603f3d011682016040523d82523d5f602084013e610a2b565b606091505b5050905080610a4d576040516312171d8360e31b815260040160405180910390fd5b50610a77565b805460038201546002830154610a77926001600160a01b0391821692911690611355565b604051339083907fc672feaa452bd52b0000f3d29c943cd9331556ab05529d49e984311220c16c19905f90a350610aba60015f805160206117ea83398151915255565b50565b610ac56111d9565b6101f4811115610ae85760405163cd4e616760e01b815260040160405180910390fd5b60035460408051918252602082018390527f5ec5620e288c4be955ccb6cfb3d55431a8fed5c4c96ffacc4b9506360695f64e910160405180910390a1600355565b610b31611205565b5f82815260016020819052604090912090600782015460ff166003811115610b5b57610b5b611597565b14610b795760405163cac0f49d60e01b815260040160405180910390fd5b60018101546001600160a01b03163314610ba657604051635a06399f60e11b815260040160405180910390fd5b8060050154600283604051602001610bc091815260200190565b60408051601f1981840301815290829052610bda91611736565b602060405180830381855afa158015610bf5573d5f803e3d5ffd5b5050506040513d601f19601f82011682018060405250810190610c189190611762565b14610c365760405163abab6bd760e01b815260040160405180910390fd5b60078101805460ff19166002179055600481015460038201545f91610c5a916117c3565b60028301549091506001600160a01b0316610d685760018201546040515f916001600160a01b03169083908381818185875af1925050503d805f8114610cbb576040519150601f19603f3d011682016040523d82523d5f602084013e610cc0565b606091505b5050905080610ce2576040516312171d8360e31b815260040160405180910390fd5b600483015415610d625760025460048401546040516001600160a01b03909216915f81818185875af1925050503d805f8114610d39576040519150601f19603f3d011682016040523d82523d5f602084013e610d3e565b606091505b50508091505080610d62576040516312171d8360e31b815260040160405180910390fd5b50610dba565b60018201546002830154610d89916001600160a01b03918216911683611355565b600482015415610dba5760028054600484015491840154610dba926001600160a01b03918216929190911690611355565b604051838152339085907fec362e2905f5025b0b88fa68647b50bdcadb528aac989973cb6ab041177bd7829060200160405180910390a35050610e0960015f805160206117ea83398151915255565b5050565b5f818152600160208190526040822090600782015460ff166003811115610e3657610e36611597565b141580610e47575080600601544210155b15610e5457505f92915050565b428160060154610e6491906117c3565b9392505050565b5f818152600160208190526040822090600782015460ff166003811115610e9457610e94611597565b148015610e6457506006015442101592915050565b610eb1611205565b60045460ff1615610ed55760405163ab35696f60e01b815260040160405180910390fd5b6001600160a01b038416610efc5760405163162908e360e11b815260040160405180910390fd5b610f0886868584611220565b610f1d6001600160a01b03851633308661138f565b5f61271060035485610f2f919061178d565b610f3991906117a4565b9050604051806101000160405280336001600160a01b03168152602001876001600160a01b03168152602001866001600160a01b0316815260200185815260200182815260200184815260200183815260200160016003811115610f9f57610f9f611597565b90525f88815260016020818152604092839020845181546001600160a01b03199081166001600160a01b03928316178355928601518285018054851691831691909117905593850151600282018054909316941693909317905560608301516003808401919091556080840151600484015560a0840151600584015560c0840151600684015560e08401516007840180549193909260ff1990921691849081111561104c5761104c611597565b021790555050604080516001600160a01b0388811682526020820188905291810184905260608101869052608081018590529088169150339089907fe584d5707a073b24a82c4414bac2dd9c326e3a1eaeb0b74ca6c885c4a20de0fc9060a00160405180910390a4506110cb60015f805160206117ea83398151915255565b505050505050565b6110db6111d9565b6001600160a01b0381166111025760405163193fd72360e11b815260040160405180910390fd5b6002546040516001600160a01b038084169216907f75b7fe723ac984bff13d3b320ed1a920035692e4a8e56fb2457774e7535c0d1d905f90a3600280546001600160a01b0319166001600160a01b0392909216919091179055565b5f818152600160208190526040822090600782015460ff16600381111561118657611186611597565b148015610e64575060060154421092915050565b6111a26111d9565b6001600160a01b0381166111d057604051631e4fbdf760e01b81525f60048201526024015b60405180910390fd5b610aba81611306565b5f546001600160a01b031633146109175760405163118cdaa760e01b81523360048201526024016111c7565b61120d6113c5565b60025f805160206117ea83398151915255565b5f8481526001602052604081206007015460ff16600381111561124557611245611597565b14611263576040516339a2986760e11b815260040160405180910390fd5b6001600160a01b03831661128a57604051631e4ec46b60e01b815260040160405180910390fd5b6103e88210156112ad5760405163162908e360e11b815260040160405180910390fd5b6112b9610e10426117d6565b8110156112d957604051637c68874160e11b815260040160405180910390fd5b6112e662278d00426117d6565b81111561090057604051637c68874160e11b815260040160405180910390fd5b5f80546001600160a01b038381166001600160a01b0319831681178455604051919092169283917f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e09190a35050565b61136283838360016113f4565b61138a57604051635274afe760e01b81526001600160a01b03841660048201526024016111c7565b505050565b61139d848484846001611456565b61090057604051635274afe760e01b81526001600160a01b03851660048201526024016111c7565b5f805160206117ea8339815191525460020361091757604051633ee5aeb560e01b815260040160405180910390fd5b60405163a9059cbb60e01b5f8181526001600160a01b038616600452602485905291602083604481808b5af1925060015f5114831661144a57838315161561143e573d5f823e3d81fd5b5f873b113d1516831692505b60405250949350505050565b6040516323b872dd60e01b5f8181526001600160a01b038781166004528616602452604485905291602083606481808c5af1925060015f511483166114b25783831516156114a6573d5f823e3d81fd5b5f883b113d1516831692505b604052505f60605295945050505050565b5f80604083850312156114d4575f80fd5b50508035926020909101359150565b5f602082840312156114f3575f80fd5b81358015158114610e64575f80fd5b80356001600160a01b0381168114611518575f80fd5b919050565b5f805f805f805f60e0888a031215611533575f80fd5b61153c88611502565b965061154a60208901611502565b955061155860408901611502565b969995985095966060810135965060808101359560a0820135955060c0909101359350915050565b5f60208284031215611590575f80fd5b5035919050565b634e487b7160e01b5f52602160045260245ffd5b600481106115c757634e487b7160e01b5f52602160045260245ffd5b9052565b5f6101008201905060018060a01b0380845116835280602085015116602084015280604085015116604084015250606083015160608301526080830151608083015260a083015160a083015260c083015160c083015260e083015161163360e08401826115ab565b5092915050565b5f805f806080858703121561164d575f80fd5b8435935061165d60208601611502565b93969395505050506040820135916060013590565b5f805f805f8060c08789031215611687575f80fd5b8635955061169760208801611502565b94506116a560408801611502565b9350606087013592506080870135915060a087013590509295509295509295565b5f602082840312156116d6575f80fd5b610e6482611502565b6001600160a01b038981168252888116602083015287166040820152606081018690526080810185905260a0810184905260c08101839052610100810161172960e08301846115ab565b9998505050505050505050565b5f82515f5b81811015611755576020818601810151858301520161173b565b505f920191825250919050565b5f60208284031215611772575f80fd5b5051919050565b634e487b7160e01b5f52601160045260245ffd5b80820281158282048414176105de576105de611779565b5f826117be57634e487b7160e01b5f52601260045260245ffd5b500490565b818103818111156105de576105de611779565b808201808211156105de576105de61177956fe9b779b17422d0df92223018b32b4d1fa46e071723d6817e2486d003becc55f00a2646970667358221220b1dca9f67dae04da15cd50d36b3959e8ecdc592f10819a4f0e17f85e1148986e64736f6c63430008140033",
}

// KlingonHTLCABI is the input ABI used to generate the binding from.
// Deprecated: Use KlingonHTLCMetaData.ABI instead.
var KlingonHTLCABI = KlingonHTLCMetaData.ABI

// KlingonHTLCBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use KlingonHTLCMetaData.Bin instead.
var KlingonHTLCBin = KlingonHTLCMetaData.Bin

// DeployKlingonHTLC deploys a new Ethereum contract, binding an instance of KlingonHTLC to it.
func DeployKlingonHTLC(auth *bind.TransactOpts, backend bind.ContractBackend, _daoAddress common.Address) (common.Address, *types.Transaction, *KlingonHTLC, error) {
	parsed, err := KlingonHTLCMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(KlingonHTLCBin), backend, _daoAddress)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &KlingonHTLC{KlingonHTLCCaller: KlingonHTLCCaller{contract: contract}, KlingonHTLCTransactor: KlingonHTLCTransactor{contract: contract}, KlingonHTLCFilterer: KlingonHTLCFilterer{contract: contract}}, nil
}

// KlingonHTLC is an auto generated Go binding around an Ethereum contract.
type KlingonHTLC struct {
	KlingonHTLCCaller     // Read-only binding to the contract
	KlingonHTLCTransactor // Write-only binding to the contract
	KlingonHTLCFilterer   // Log filterer for contract events
}

// KlingonHTLCCaller is an auto generated read-only Go binding around an Ethereum contract.
type KlingonHTLCCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// KlingonHTLCTransactor is an auto generated write-only Go binding around an Ethereum contract.
type KlingonHTLCTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// KlingonHTLCFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type KlingonHTLCFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// KlingonHTLCSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type KlingonHTLCSession struct {
	Contract     *KlingonHTLC      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// KlingonHTLCCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type KlingonHTLCCallerSession struct {
	Contract *KlingonHTLCCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// KlingonHTLCTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type KlingonHTLCTransactorSession struct {
	Contract     *KlingonHTLCTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// KlingonHTLCRaw is an auto generated low-level Go binding around an Ethereum contract.
type KlingonHTLCRaw struct {
	Contract *KlingonHTLC // Generic contract binding to access the raw methods on
}

// KlingonHTLCCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type KlingonHTLCCallerRaw struct {
	Contract *KlingonHTLCCaller // Generic read-only contract binding to access the raw methods on
}

// KlingonHTLCTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type KlingonHTLCTransactorRaw struct {
	Contract *KlingonHTLCTransactor // Generic write-only contract binding to access the raw methods on
}

// NewKlingonHTLC creates a new instance of KlingonHTLC, bound to a specific deployed contract.
func NewKlingonHTLC(address common.Address, backend bind.ContractBackend) (*KlingonHTLC, error) {
	contract, err := bindKlingonHTLC(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLC{KlingonHTLCCaller: KlingonHTLCCaller{contract: contract}, KlingonHTLCTransactor: KlingonHTLCTransactor{contract: contract}, KlingonHTLCFilterer: KlingonHTLCFilterer{contract: contract}}, nil
}

// NewKlingonHTLCCaller creates a new read-only instance of KlingonHTLC, bound to a specific deployed contract.
func NewKlingonHTLCCaller(address common.Address, caller bind.ContractCaller) (*KlingonHTLCCaller, error) {
	contract, err := bindKlingonHTLC(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCCaller{contract: contract}, nil
}

// NewKlingonHTLCTransactor creates a new write-only instance of KlingonHTLC, bound to a specific deployed contract.
func NewKlingonHTLCTransactor(address common.Address, transactor bind.ContractTransactor) (*KlingonHTLCTransactor, error) {
	contract, err := bindKlingonHTLC(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCTransactor{contract: contract}, nil
}

// NewKlingonHTLCFilterer creates a new log filterer instance of KlingonHTLC, bound to a specific deployed contract.
func NewKlingonHTLCFilterer(address common.Address, filterer bind.ContractFilterer) (*KlingonHTLCFilterer, error) {
	contract, err := bindKlingonHTLC(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCFilterer{contract: contract}, nil
}

// bindKlingonHTLC binds a generic wrapper to an already deployed contract.
func bindKlingonHTLC(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := KlingonHTLCMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_KlingonHTLC *KlingonHTLCRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _KlingonHTLC.Contract.KlingonHTLCCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_KlingonHTLC *KlingonHTLCRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.KlingonHTLCTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_KlingonHTLC *KlingonHTLCRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.KlingonHTLCTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_KlingonHTLC *KlingonHTLCCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _KlingonHTLC.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_KlingonHTLC *KlingonHTLCTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_KlingonHTLC *KlingonHTLCTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.contract.Transact(opts, method, params...)
}

// FEEDENOMINATOR is a free data retrieval call binding the contract method 0xd73792a9.
//
// Solidity: function FEE_DENOMINATOR() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) FEEDENOMINATOR(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "FEE_DENOMINATOR")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// FEEDENOMINATOR is a free data retrieval call binding the contract method 0xd73792a9.
//
// Solidity: function FEE_DENOMINATOR() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) FEEDENOMINATOR() (*big.Int, error) {
	return _KlingonHTLC.Contract.FEEDENOMINATOR(&_KlingonHTLC.CallOpts)
}

// FEEDENOMINATOR is a free data retrieval call binding the contract method 0xd73792a9.
//
// Solidity: function FEE_DENOMINATOR() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) FEEDENOMINATOR() (*big.Int, error) {
	return _KlingonHTLC.Contract.FEEDENOMINATOR(&_KlingonHTLC.CallOpts)
}

// MAXFEEBPS is a free data retrieval call binding the contract method 0xd55be8c6.
//
// Solidity: function MAX_FEE_BPS() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) MAXFEEBPS(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "MAX_FEE_BPS")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MAXFEEBPS is a free data retrieval call binding the contract method 0xd55be8c6.
//
// Solidity: function MAX_FEE_BPS() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) MAXFEEBPS() (*big.Int, error) {
	return _KlingonHTLC.Contract.MAXFEEBPS(&_KlingonHTLC.CallOpts)
}

// MAXFEEBPS is a free data retrieval call binding the contract method 0xd55be8c6.
//
// Solidity: function MAX_FEE_BPS() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) MAXFEEBPS() (*big.Int, error) {
	return _KlingonHTLC.Contract.MAXFEEBPS(&_KlingonHTLC.CallOpts)
}

// MAXTIMELOCK is a free data retrieval call binding the contract method 0x94f1825b.
//
// Solidity: function MAX_TIMELOCK() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) MAXTIMELOCK(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "MAX_TIMELOCK")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MAXTIMELOCK is a free data retrieval call binding the contract method 0x94f1825b.
//
// Solidity: function MAX_TIMELOCK() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) MAXTIMELOCK() (*big.Int, error) {
	return _KlingonHTLC.Contract.MAXTIMELOCK(&_KlingonHTLC.CallOpts)
}

// MAXTIMELOCK is a free data retrieval call binding the contract method 0x94f1825b.
//
// Solidity: function MAX_TIMELOCK() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) MAXTIMELOCK() (*big.Int, error) {
	return _KlingonHTLC.Contract.MAXTIMELOCK(&_KlingonHTLC.CallOpts)
}

// MINSWAPAMOUNT is a free data retrieval call binding the contract method 0x99f34c12.
//
// Solidity: function MIN_SWAP_AMOUNT() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) MINSWAPAMOUNT(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "MIN_SWAP_AMOUNT")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MINSWAPAMOUNT is a free data retrieval call binding the contract method 0x99f34c12.
//
// Solidity: function MIN_SWAP_AMOUNT() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) MINSWAPAMOUNT() (*big.Int, error) {
	return _KlingonHTLC.Contract.MINSWAPAMOUNT(&_KlingonHTLC.CallOpts)
}

// MINSWAPAMOUNT is a free data retrieval call binding the contract method 0x99f34c12.
//
// Solidity: function MIN_SWAP_AMOUNT() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) MINSWAPAMOUNT() (*big.Int, error) {
	return _KlingonHTLC.Contract.MINSWAPAMOUNT(&_KlingonHTLC.CallOpts)
}

// MINTIMELOCK is a free data retrieval call binding the contract method 0xfb0c7549.
//
// Solidity: function MIN_TIMELOCK() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) MINTIMELOCK(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "MIN_TIMELOCK")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MINTIMELOCK is a free data retrieval call binding the contract method 0xfb0c7549.
//
// Solidity: function MIN_TIMELOCK() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) MINTIMELOCK() (*big.Int, error) {
	return _KlingonHTLC.Contract.MINTIMELOCK(&_KlingonHTLC.CallOpts)
}

// MINTIMELOCK is a free data retrieval call binding the contract method 0xfb0c7549.
//
// Solidity: function MIN_TIMELOCK() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) MINTIMELOCK() (*big.Int, error) {
	return _KlingonHTLC.Contract.MINTIMELOCK(&_KlingonHTLC.CallOpts)
}

// CanClaim is a free data retrieval call binding the contract method 0xeaaa98fa.
//
// Solidity: function canClaim(bytes32 swapId) view returns(bool)
func (_KlingonHTLC *KlingonHTLCCaller) CanClaim(opts *bind.CallOpts, swapId [32]byte) (bool, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "canClaim", swapId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanClaim is a free data retrieval call binding the contract method 0xeaaa98fa.
//
// Solidity: function canClaim(bytes32 swapId) view returns(bool)
func (_KlingonHTLC *KlingonHTLCSession) CanClaim(swapId [32]byte) (bool, error) {
	return _KlingonHTLC.Contract.CanClaim(&_KlingonHTLC.CallOpts, swapId)
}

// CanClaim is a free data retrieval call binding the contract method 0xeaaa98fa.
//
// Solidity: function canClaim(bytes32 swapId) view returns(bool)
func (_KlingonHTLC *KlingonHTLCCallerSession) CanClaim(swapId [32]byte) (bool, error) {
	return _KlingonHTLC.Contract.CanClaim(&_KlingonHTLC.CallOpts, swapId)
}

// CanRefund is a free data retrieval call binding the contract method 0x8c8c2003.
//
// Solidity: function canRefund(bytes32 swapId) view returns(bool)
func (_KlingonHTLC *KlingonHTLCCaller) CanRefund(opts *bind.CallOpts, swapId [32]byte) (bool, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "canRefund", swapId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanRefund is a free data retrieval call binding the contract method 0x8c8c2003.
//
// Solidity: function canRefund(bytes32 swapId) view returns(bool)
func (_KlingonHTLC *KlingonHTLCSession) CanRefund(swapId [32]byte) (bool, error) {
	return _KlingonHTLC.Contract.CanRefund(&_KlingonHTLC.CallOpts, swapId)
}

// CanRefund is a free data retrieval call binding the contract method 0x8c8c2003.
//
// Solidity: function canRefund(bytes32 swapId) view returns(bool)
func (_KlingonHTLC *KlingonHTLCCallerSession) CanRefund(swapId [32]byte) (bool, error) {
	return _KlingonHTLC.Contract.CanRefund(&_KlingonHTLC.CallOpts, swapId)
}

// ComputeSwapId is a free data retrieval call binding the contract method 0x2554c6e3.
//
// Solidity: function computeSwapId(address sender, address receiver, address token, uint256 amount, bytes32 secretHash, uint256 timelock, uint256 nonce) view returns(bytes32)
func (_KlingonHTLC *KlingonHTLCCaller) ComputeSwapId(opts *bind.CallOpts, sender common.Address, receiver common.Address, token common.Address, amount *big.Int, secretHash [32]byte, timelock *big.Int, nonce *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "computeSwapId", sender, receiver, token, amount, secretHash, timelock, nonce)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ComputeSwapId is a free data retrieval call binding the contract method 0x2554c6e3.
//
// Solidity: function computeSwapId(address sender, address receiver, address token, uint256 amount, bytes32 secretHash, uint256 timelock, uint256 nonce) view returns(bytes32)
func (_KlingonHTLC *KlingonHTLCSession) ComputeSwapId(sender common.Address, receiver common.Address, token common.Address, amount *big.Int, secretHash [32]byte, timelock *big.Int, nonce *big.Int) ([32]byte, error) {
	return _KlingonHTLC.Contract.ComputeSwapId(&_KlingonHTLC.CallOpts, sender, receiver, token, amount, secretHash, timelock, nonce)
}

// ComputeSwapId is a free data retrieval call binding the contract method 0x2554c6e3.
//
// Solidity: function computeSwapId(address sender, address receiver, address token, uint256 amount, bytes32 secretHash, uint256 timelock, uint256 nonce) view returns(bytes32)
func (_KlingonHTLC *KlingonHTLCCallerSession) ComputeSwapId(sender common.Address, receiver common.Address, token common.Address, amount *big.Int, secretHash [32]byte, timelock *big.Int, nonce *big.Int) ([32]byte, error) {
	return _KlingonHTLC.Contract.ComputeSwapId(&_KlingonHTLC.CallOpts, sender, receiver, token, amount, secretHash, timelock, nonce)
}

// DaoAddress is a free data retrieval call binding the contract method 0x2131c68c.
//
// Solidity: function daoAddress() view returns(address)
func (_KlingonHTLC *KlingonHTLCCaller) DaoAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "daoAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// DaoAddress is a free data retrieval call binding the contract method 0x2131c68c.
//
// Solidity: function daoAddress() view returns(address)
func (_KlingonHTLC *KlingonHTLCSession) DaoAddress() (common.Address, error) {
	return _KlingonHTLC.Contract.DaoAddress(&_KlingonHTLC.CallOpts)
}

// DaoAddress is a free data retrieval call binding the contract method 0x2131c68c.
//
// Solidity: function daoAddress() view returns(address)
func (_KlingonHTLC *KlingonHTLCCallerSession) DaoAddress() (common.Address, error) {
	return _KlingonHTLC.Contract.DaoAddress(&_KlingonHTLC.CallOpts)
}

// FeeBps is a free data retrieval call binding the contract method 0x24a9d853.
//
// Solidity: function feeBps() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) FeeBps(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "feeBps")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// FeeBps is a free data retrieval call binding the contract method 0x24a9d853.
//
// Solidity: function feeBps() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) FeeBps() (*big.Int, error) {
	return _KlingonHTLC.Contract.FeeBps(&_KlingonHTLC.CallOpts)
}

// FeeBps is a free data retrieval call binding the contract method 0x24a9d853.
//
// Solidity: function feeBps() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) FeeBps() (*big.Int, error) {
	return _KlingonHTLC.Contract.FeeBps(&_KlingonHTLC.CallOpts)
}

// GetChainId is a free data retrieval call binding the contract method 0x3408e470.
//
// Solidity: function getChainId() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) GetChainId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "getChainId")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetChainId is a free data retrieval call binding the contract method 0x3408e470.
//
// Solidity: function getChainId() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) GetChainId() (*big.Int, error) {
	return _KlingonHTLC.Contract.GetChainId(&_KlingonHTLC.CallOpts)
}

// GetChainId is a free data retrieval call binding the contract method 0x3408e470.
//
// Solidity: function getChainId() view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) GetChainId() (*big.Int, error) {
	return _KlingonHTLC.Contract.GetChainId(&_KlingonHTLC.CallOpts)
}

// GetSwap is a free data retrieval call binding the contract method 0x3da0e66e.
//
// Solidity: function getSwap(bytes32 swapId) view returns((address,address,address,uint256,uint256,bytes32,uint256,uint8))
func (_KlingonHTLC *KlingonHTLCCaller) GetSwap(opts *bind.CallOpts, swapId [32]byte) (KlingonHTLCSwap, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "getSwap", swapId)

	if err != nil {
		return *new(KlingonHTLCSwap), err
	}

	out0 := *abi.ConvertType(out[0], new(KlingonHTLCSwap)).(*KlingonHTLCSwap)

	return out0, err

}

// GetSwap is a free data retrieval call binding the contract method 0x3da0e66e.
//
// Solidity: function getSwap(bytes32 swapId) view returns((address,address,address,uint256,uint256,bytes32,uint256,uint8))
func (_KlingonHTLC *KlingonHTLCSession) GetSwap(swapId [32]byte) (KlingonHTLCSwap, error) {
	return _KlingonHTLC.Contract.GetSwap(&_KlingonHTLC.CallOpts, swapId)
}

// GetSwap is a free data retrieval call binding the contract method 0x3da0e66e.
//
// Solidity: function getSwap(bytes32 swapId) view returns((address,address,address,uint256,uint256,bytes32,uint256,uint8))
func (_KlingonHTLC *KlingonHTLCCallerSession) GetSwap(swapId [32]byte) (KlingonHTLCSwap, error) {
	return _KlingonHTLC.Contract.GetSwap(&_KlingonHTLC.CallOpts, swapId)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_KlingonHTLC *KlingonHTLCCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_KlingonHTLC *KlingonHTLCSession) Owner() (common.Address, error) {
	return _KlingonHTLC.Contract.Owner(&_KlingonHTLC.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_KlingonHTLC *KlingonHTLCCallerSession) Owner() (common.Address, error) {
	return _KlingonHTLC.Contract.Owner(&_KlingonHTLC.CallOpts)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_KlingonHTLC *KlingonHTLCCaller) Paused(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "paused")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_KlingonHTLC *KlingonHTLCSession) Paused() (bool, error) {
	return _KlingonHTLC.Contract.Paused(&_KlingonHTLC.CallOpts)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_KlingonHTLC *KlingonHTLCCallerSession) Paused() (bool, error) {
	return _KlingonHTLC.Contract.Paused(&_KlingonHTLC.CallOpts)
}

// Swaps is a free data retrieval call binding the contract method 0xeb84e7f2.
//
// Solidity: function swaps(bytes32 ) view returns(address sender, address receiver, address token, uint256 amount, uint256 daoFee, bytes32 secretHash, uint256 timelock, uint8 state)
func (_KlingonHTLC *KlingonHTLCCaller) Swaps(opts *bind.CallOpts, arg0 [32]byte) (struct {
	Sender     common.Address
	Receiver   common.Address
	Token      common.Address
	Amount     *big.Int
	DaoFee     *big.Int
	SecretHash [32]byte
	Timelock   *big.Int
	State      uint8
}, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "swaps", arg0)

	outstruct := new(struct {
		Sender     common.Address
		Receiver   common.Address
		Token      common.Address
		Amount     *big.Int
		DaoFee     *big.Int
		SecretHash [32]byte
		Timelock   *big.Int
		State      uint8
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Sender = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.Receiver = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.Token = *abi.ConvertType(out[2], new(common.Address)).(*common.Address)
	outstruct.Amount = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.DaoFee = *abi.ConvertType(out[4], new(*big.Int)).(**big.Int)
	outstruct.SecretHash = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Timelock = *abi.ConvertType(out[6], new(*big.Int)).(**big.Int)
	outstruct.State = *abi.ConvertType(out[7], new(uint8)).(*uint8)

	return *outstruct, err

}

// Swaps is a free data retrieval call binding the contract method 0xeb84e7f2.
//
// Solidity: function swaps(bytes32 ) view returns(address sender, address receiver, address token, uint256 amount, uint256 daoFee, bytes32 secretHash, uint256 timelock, uint8 state)
func (_KlingonHTLC *KlingonHTLCSession) Swaps(arg0 [32]byte) (struct {
	Sender     common.Address
	Receiver   common.Address
	Token      common.Address
	Amount     *big.Int
	DaoFee     *big.Int
	SecretHash [32]byte
	Timelock   *big.Int
	State      uint8
}, error) {
	return _KlingonHTLC.Contract.Swaps(&_KlingonHTLC.CallOpts, arg0)
}

// Swaps is a free data retrieval call binding the contract method 0xeb84e7f2.
//
// Solidity: function swaps(bytes32 ) view returns(address sender, address receiver, address token, uint256 amount, uint256 daoFee, bytes32 secretHash, uint256 timelock, uint8 state)
func (_KlingonHTLC *KlingonHTLCCallerSession) Swaps(arg0 [32]byte) (struct {
	Sender     common.Address
	Receiver   common.Address
	Token      common.Address
	Amount     *big.Int
	DaoFee     *big.Int
	SecretHash [32]byte
	Timelock   *big.Int
	State      uint8
}, error) {
	return _KlingonHTLC.Contract.Swaps(&_KlingonHTLC.CallOpts, arg0)
}

// TimeUntilRefund is a free data retrieval call binding the contract method 0x89cdebe3.
//
// Solidity: function timeUntilRefund(bytes32 swapId) view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCaller) TimeUntilRefund(opts *bind.CallOpts, swapId [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "timeUntilRefund", swapId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TimeUntilRefund is a free data retrieval call binding the contract method 0x89cdebe3.
//
// Solidity: function timeUntilRefund(bytes32 swapId) view returns(uint256)
func (_KlingonHTLC *KlingonHTLCSession) TimeUntilRefund(swapId [32]byte) (*big.Int, error) {
	return _KlingonHTLC.Contract.TimeUntilRefund(&_KlingonHTLC.CallOpts, swapId)
}

// TimeUntilRefund is a free data retrieval call binding the contract method 0x89cdebe3.
//
// Solidity: function timeUntilRefund(bytes32 swapId) view returns(uint256)
func (_KlingonHTLC *KlingonHTLCCallerSession) TimeUntilRefund(swapId [32]byte) (*big.Int, error) {
	return _KlingonHTLC.Contract.TimeUntilRefund(&_KlingonHTLC.CallOpts, swapId)
}

// VerifySecret is a free data retrieval call binding the contract method 0x11758b14.
//
// Solidity: function verifySecret(bytes32 swapId, bytes32 secret) view returns(bool)
func (_KlingonHTLC *KlingonHTLCCaller) VerifySecret(opts *bind.CallOpts, swapId [32]byte, secret [32]byte) (bool, error) {
	var out []interface{}
	err := _KlingonHTLC.contract.Call(opts, &out, "verifySecret", swapId, secret)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// VerifySecret is a free data retrieval call binding the contract method 0x11758b14.
//
// Solidity: function verifySecret(bytes32 swapId, bytes32 secret) view returns(bool)
func (_KlingonHTLC *KlingonHTLCSession) VerifySecret(swapId [32]byte, secret [32]byte) (bool, error) {
	return _KlingonHTLC.Contract.VerifySecret(&_KlingonHTLC.CallOpts, swapId, secret)
}

// VerifySecret is a free data retrieval call binding the contract method 0x11758b14.
//
// Solidity: function verifySecret(bytes32 swapId, bytes32 secret) view returns(bool)
func (_KlingonHTLC *KlingonHTLCCallerSession) VerifySecret(swapId [32]byte, secret [32]byte) (bool, error) {
	return _KlingonHTLC.Contract.VerifySecret(&_KlingonHTLC.CallOpts, swapId, secret)
}

// Claim is a paid mutator transaction binding the contract method 0x84cc9dfb.
//
// Solidity: function claim(bytes32 swapId, bytes32 secret) returns()
func (_KlingonHTLC *KlingonHTLCTransactor) Claim(opts *bind.TransactOpts, swapId [32]byte, secret [32]byte) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "claim", swapId, secret)
}

// Claim is a paid mutator transaction binding the contract method 0x84cc9dfb.
//
// Solidity: function claim(bytes32 swapId, bytes32 secret) returns()
func (_KlingonHTLC *KlingonHTLCSession) Claim(swapId [32]byte, secret [32]byte) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.Claim(&_KlingonHTLC.TransactOpts, swapId, secret)
}

// Claim is a paid mutator transaction binding the contract method 0x84cc9dfb.
//
// Solidity: function claim(bytes32 swapId, bytes32 secret) returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) Claim(swapId [32]byte, secret [32]byte) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.Claim(&_KlingonHTLC.TransactOpts, swapId, secret)
}

// CreateSwapERC20 is a paid mutator transaction binding the contract method 0x95824c68.
//
// Solidity: function createSwapERC20(bytes32 swapId, address receiver, address token, uint256 amount, bytes32 secretHash, uint256 timelock) returns()
func (_KlingonHTLC *KlingonHTLCTransactor) CreateSwapERC20(opts *bind.TransactOpts, swapId [32]byte, receiver common.Address, token common.Address, amount *big.Int, secretHash [32]byte, timelock *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "createSwapERC20", swapId, receiver, token, amount, secretHash, timelock)
}

// CreateSwapERC20 is a paid mutator transaction binding the contract method 0x95824c68.
//
// Solidity: function createSwapERC20(bytes32 swapId, address receiver, address token, uint256 amount, bytes32 secretHash, uint256 timelock) returns()
func (_KlingonHTLC *KlingonHTLCSession) CreateSwapERC20(swapId [32]byte, receiver common.Address, token common.Address, amount *big.Int, secretHash [32]byte, timelock *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.CreateSwapERC20(&_KlingonHTLC.TransactOpts, swapId, receiver, token, amount, secretHash, timelock)
}

// CreateSwapERC20 is a paid mutator transaction binding the contract method 0x95824c68.
//
// Solidity: function createSwapERC20(bytes32 swapId, address receiver, address token, uint256 amount, bytes32 secretHash, uint256 timelock) returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) CreateSwapERC20(swapId [32]byte, receiver common.Address, token common.Address, amount *big.Int, secretHash [32]byte, timelock *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.CreateSwapERC20(&_KlingonHTLC.TransactOpts, swapId, receiver, token, amount, secretHash, timelock)
}

// CreateSwapNative is a paid mutator transaction binding the contract method 0x44248c2c.
//
// Solidity: function createSwapNative(bytes32 swapId, address receiver, bytes32 secretHash, uint256 timelock) payable returns()
func (_KlingonHTLC *KlingonHTLCTransactor) CreateSwapNative(opts *bind.TransactOpts, swapId [32]byte, receiver common.Address, secretHash [32]byte, timelock *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "createSwapNative", swapId, receiver, secretHash, timelock)
}

// CreateSwapNative is a paid mutator transaction binding the contract method 0x44248c2c.
//
// Solidity: function createSwapNative(bytes32 swapId, address receiver, bytes32 secretHash, uint256 timelock) payable returns()
func (_KlingonHTLC *KlingonHTLCSession) CreateSwapNative(swapId [32]byte, receiver common.Address, secretHash [32]byte, timelock *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.CreateSwapNative(&_KlingonHTLC.TransactOpts, swapId, receiver, secretHash, timelock)
}

// CreateSwapNative is a paid mutator transaction binding the contract method 0x44248c2c.
//
// Solidity: function createSwapNative(bytes32 swapId, address receiver, bytes32 secretHash, uint256 timelock) payable returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) CreateSwapNative(swapId [32]byte, receiver common.Address, secretHash [32]byte, timelock *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.CreateSwapNative(&_KlingonHTLC.TransactOpts, swapId, receiver, secretHash, timelock)
}

// Refund is a paid mutator transaction binding the contract method 0x7249fbb6.
//
// Solidity: function refund(bytes32 swapId) returns()
func (_KlingonHTLC *KlingonHTLCTransactor) Refund(opts *bind.TransactOpts, swapId [32]byte) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "refund", swapId)
}

// Refund is a paid mutator transaction binding the contract method 0x7249fbb6.
//
// Solidity: function refund(bytes32 swapId) returns()
func (_KlingonHTLC *KlingonHTLCSession) Refund(swapId [32]byte) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.Refund(&_KlingonHTLC.TransactOpts, swapId)
}

// Refund is a paid mutator transaction binding the contract method 0x7249fbb6.
//
// Solidity: function refund(bytes32 swapId) returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) Refund(swapId [32]byte) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.Refund(&_KlingonHTLC.TransactOpts, swapId)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_KlingonHTLC *KlingonHTLCTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_KlingonHTLC *KlingonHTLCSession) RenounceOwnership() (*types.Transaction, error) {
	return _KlingonHTLC.Contract.RenounceOwnership(&_KlingonHTLC.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _KlingonHTLC.Contract.RenounceOwnership(&_KlingonHTLC.TransactOpts)
}

// SetDaoAddress is a paid mutator transaction binding the contract method 0x9a3cac6a.
//
// Solidity: function setDaoAddress(address _daoAddress) returns()
func (_KlingonHTLC *KlingonHTLCTransactor) SetDaoAddress(opts *bind.TransactOpts, _daoAddress common.Address) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "setDaoAddress", _daoAddress)
}

// SetDaoAddress is a paid mutator transaction binding the contract method 0x9a3cac6a.
//
// Solidity: function setDaoAddress(address _daoAddress) returns()
func (_KlingonHTLC *KlingonHTLCSession) SetDaoAddress(_daoAddress common.Address) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.SetDaoAddress(&_KlingonHTLC.TransactOpts, _daoAddress)
}

// SetDaoAddress is a paid mutator transaction binding the contract method 0x9a3cac6a.
//
// Solidity: function setDaoAddress(address _daoAddress) returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) SetDaoAddress(_daoAddress common.Address) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.SetDaoAddress(&_KlingonHTLC.TransactOpts, _daoAddress)
}

// SetFeeBps is a paid mutator transaction binding the contract method 0x72c27b62.
//
// Solidity: function setFeeBps(uint256 _feeBps) returns()
func (_KlingonHTLC *KlingonHTLCTransactor) SetFeeBps(opts *bind.TransactOpts, _feeBps *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "setFeeBps", _feeBps)
}

// SetFeeBps is a paid mutator transaction binding the contract method 0x72c27b62.
//
// Solidity: function setFeeBps(uint256 _feeBps) returns()
func (_KlingonHTLC *KlingonHTLCSession) SetFeeBps(_feeBps *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.SetFeeBps(&_KlingonHTLC.TransactOpts, _feeBps)
}

// SetFeeBps is a paid mutator transaction binding the contract method 0x72c27b62.
//
// Solidity: function setFeeBps(uint256 _feeBps) returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) SetFeeBps(_feeBps *big.Int) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.SetFeeBps(&_KlingonHTLC.TransactOpts, _feeBps)
}

// SetPaused is a paid mutator transaction binding the contract method 0x16c38b3c.
//
// Solidity: function setPaused(bool _paused) returns()
func (_KlingonHTLC *KlingonHTLCTransactor) SetPaused(opts *bind.TransactOpts, _paused bool) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "setPaused", _paused)
}

// SetPaused is a paid mutator transaction binding the contract method 0x16c38b3c.
//
// Solidity: function setPaused(bool _paused) returns()
func (_KlingonHTLC *KlingonHTLCSession) SetPaused(_paused bool) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.SetPaused(&_KlingonHTLC.TransactOpts, _paused)
}

// SetPaused is a paid mutator transaction binding the contract method 0x16c38b3c.
//
// Solidity: function setPaused(bool _paused) returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) SetPaused(_paused bool) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.SetPaused(&_KlingonHTLC.TransactOpts, _paused)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_KlingonHTLC *KlingonHTLCTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _KlingonHTLC.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_KlingonHTLC *KlingonHTLCSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.TransferOwnership(&_KlingonHTLC.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_KlingonHTLC *KlingonHTLCTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _KlingonHTLC.Contract.TransferOwnership(&_KlingonHTLC.TransactOpts, newOwner)
}

// KlingonHTLCDaoAddressUpdatedIterator is returned from FilterDaoAddressUpdated and is used to iterate over the raw logs and unpacked data for DaoAddressUpdated events raised by the KlingonHTLC contract.
type KlingonHTLCDaoAddressUpdatedIterator struct {
	Event *KlingonHTLCDaoAddressUpdated // Event containing the contract specifics and raw log

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
func (it *KlingonHTLCDaoAddressUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(KlingonHTLCDaoAddressUpdated)
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
		it.Event = new(KlingonHTLCDaoAddressUpdated)
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
func (it *KlingonHTLCDaoAddressUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *KlingonHTLCDaoAddressUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// KlingonHTLCDaoAddressUpdated represents a DaoAddressUpdated event raised by the KlingonHTLC contract.
type KlingonHTLCDaoAddressUpdated struct {
	OldDao common.Address
	NewDao common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterDaoAddressUpdated is a free log retrieval operation binding the contract event 0x75b7fe723ac984bff13d3b320ed1a920035692e4a8e56fb2457774e7535c0d1d.
//
// Solidity: event DaoAddressUpdated(address indexed oldDao, address indexed newDao)
func (_KlingonHTLC *KlingonHTLCFilterer) FilterDaoAddressUpdated(opts *bind.FilterOpts, oldDao []common.Address, newDao []common.Address) (*KlingonHTLCDaoAddressUpdatedIterator, error) {

	var oldDaoRule []interface{}
	for _, oldDaoItem := range oldDao {
		oldDaoRule = append(oldDaoRule, oldDaoItem)
	}
	var newDaoRule []interface{}
	for _, newDaoItem := range newDao {
		newDaoRule = append(newDaoRule, newDaoItem)
	}

	logs, sub, err := _KlingonHTLC.contract.FilterLogs(opts, "DaoAddressUpdated", oldDaoRule, newDaoRule)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCDaoAddressUpdatedIterator{contract: _KlingonHTLC.contract, event: "DaoAddressUpdated", logs: logs, sub: sub}, nil
}

// WatchDaoAddressUpdated is a free log subscription operation binding the contract event 0x75b7fe723ac984bff13d3b320ed1a920035692e4a8e56fb2457774e7535c0d1d.
//
// Solidity: event DaoAddressUpdated(address indexed oldDao, address indexed newDao)
func (_KlingonHTLC *KlingonHTLCFilterer) WatchDaoAddressUpdated(opts *bind.WatchOpts, sink chan<- *KlingonHTLCDaoAddressUpdated, oldDao []common.Address, newDao []common.Address) (event.Subscription, error) {

	var oldDaoRule []interface{}
	for _, oldDaoItem := range oldDao {
		oldDaoRule = append(oldDaoRule, oldDaoItem)
	}
	var newDaoRule []interface{}
	for _, newDaoItem := range newDao {
		newDaoRule = append(newDaoRule, newDaoItem)
	}

	logs, sub, err := _KlingonHTLC.contract.WatchLogs(opts, "DaoAddressUpdated", oldDaoRule, newDaoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(KlingonHTLCDaoAddressUpdated)
				if err := _KlingonHTLC.contract.UnpackLog(event, "DaoAddressUpdated", log); err != nil {
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

// ParseDaoAddressUpdated is a log parse operation binding the contract event 0x75b7fe723ac984bff13d3b320ed1a920035692e4a8e56fb2457774e7535c0d1d.
//
// Solidity: event DaoAddressUpdated(address indexed oldDao, address indexed newDao)
func (_KlingonHTLC *KlingonHTLCFilterer) ParseDaoAddressUpdated(log types.Log) (*KlingonHTLCDaoAddressUpdated, error) {
	event := new(KlingonHTLCDaoAddressUpdated)
	if err := _KlingonHTLC.contract.UnpackLog(event, "DaoAddressUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// KlingonHTLCFeeBpsUpdatedIterator is returned from FilterFeeBpsUpdated and is used to iterate over the raw logs and unpacked data for FeeBpsUpdated events raised by the KlingonHTLC contract.
type KlingonHTLCFeeBpsUpdatedIterator struct {
	Event *KlingonHTLCFeeBpsUpdated // Event containing the contract specifics and raw log

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
func (it *KlingonHTLCFeeBpsUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(KlingonHTLCFeeBpsUpdated)
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
		it.Event = new(KlingonHTLCFeeBpsUpdated)
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
func (it *KlingonHTLCFeeBpsUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *KlingonHTLCFeeBpsUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// KlingonHTLCFeeBpsUpdated represents a FeeBpsUpdated event raised by the KlingonHTLC contract.
type KlingonHTLCFeeBpsUpdated struct {
	OldFee *big.Int
	NewFee *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterFeeBpsUpdated is a free log retrieval operation binding the contract event 0x5ec5620e288c4be955ccb6cfb3d55431a8fed5c4c96ffacc4b9506360695f64e.
//
// Solidity: event FeeBpsUpdated(uint256 oldFee, uint256 newFee)
func (_KlingonHTLC *KlingonHTLCFilterer) FilterFeeBpsUpdated(opts *bind.FilterOpts) (*KlingonHTLCFeeBpsUpdatedIterator, error) {

	logs, sub, err := _KlingonHTLC.contract.FilterLogs(opts, "FeeBpsUpdated")
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCFeeBpsUpdatedIterator{contract: _KlingonHTLC.contract, event: "FeeBpsUpdated", logs: logs, sub: sub}, nil
}

// WatchFeeBpsUpdated is a free log subscription operation binding the contract event 0x5ec5620e288c4be955ccb6cfb3d55431a8fed5c4c96ffacc4b9506360695f64e.
//
// Solidity: event FeeBpsUpdated(uint256 oldFee, uint256 newFee)
func (_KlingonHTLC *KlingonHTLCFilterer) WatchFeeBpsUpdated(opts *bind.WatchOpts, sink chan<- *KlingonHTLCFeeBpsUpdated) (event.Subscription, error) {

	logs, sub, err := _KlingonHTLC.contract.WatchLogs(opts, "FeeBpsUpdated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(KlingonHTLCFeeBpsUpdated)
				if err := _KlingonHTLC.contract.UnpackLog(event, "FeeBpsUpdated", log); err != nil {
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

// ParseFeeBpsUpdated is a log parse operation binding the contract event 0x5ec5620e288c4be955ccb6cfb3d55431a8fed5c4c96ffacc4b9506360695f64e.
//
// Solidity: event FeeBpsUpdated(uint256 oldFee, uint256 newFee)
func (_KlingonHTLC *KlingonHTLCFilterer) ParseFeeBpsUpdated(log types.Log) (*KlingonHTLCFeeBpsUpdated, error) {
	event := new(KlingonHTLCFeeBpsUpdated)
	if err := _KlingonHTLC.contract.UnpackLog(event, "FeeBpsUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// KlingonHTLCOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the KlingonHTLC contract.
type KlingonHTLCOwnershipTransferredIterator struct {
	Event *KlingonHTLCOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *KlingonHTLCOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(KlingonHTLCOwnershipTransferred)
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
		it.Event = new(KlingonHTLCOwnershipTransferred)
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
func (it *KlingonHTLCOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *KlingonHTLCOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// KlingonHTLCOwnershipTransferred represents a OwnershipTransferred event raised by the KlingonHTLC contract.
type KlingonHTLCOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_KlingonHTLC *KlingonHTLCFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*KlingonHTLCOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _KlingonHTLC.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCOwnershipTransferredIterator{contract: _KlingonHTLC.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_KlingonHTLC *KlingonHTLCFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *KlingonHTLCOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _KlingonHTLC.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(KlingonHTLCOwnershipTransferred)
				if err := _KlingonHTLC.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_KlingonHTLC *KlingonHTLCFilterer) ParseOwnershipTransferred(log types.Log) (*KlingonHTLCOwnershipTransferred, error) {
	event := new(KlingonHTLCOwnershipTransferred)
	if err := _KlingonHTLC.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// KlingonHTLCPausedIterator is returned from FilterPaused and is used to iterate over the raw logs and unpacked data for Paused events raised by the KlingonHTLC contract.
type KlingonHTLCPausedIterator struct {
	Event *KlingonHTLCPaused // Event containing the contract specifics and raw log

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
func (it *KlingonHTLCPausedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(KlingonHTLCPaused)
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
		it.Event = new(KlingonHTLCPaused)
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
func (it *KlingonHTLCPausedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *KlingonHTLCPausedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// KlingonHTLCPaused represents a Paused event raised by the KlingonHTLC contract.
type KlingonHTLCPaused struct {
	IsPaused bool
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterPaused is a free log retrieval operation binding the contract event 0x0e2fb031ee032dc02d8011dc50b816eb450cf856abd8261680dac74f72165bd2.
//
// Solidity: event Paused(bool isPaused)
func (_KlingonHTLC *KlingonHTLCFilterer) FilterPaused(opts *bind.FilterOpts) (*KlingonHTLCPausedIterator, error) {

	logs, sub, err := _KlingonHTLC.contract.FilterLogs(opts, "Paused")
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCPausedIterator{contract: _KlingonHTLC.contract, event: "Paused", logs: logs, sub: sub}, nil
}

// WatchPaused is a free log subscription operation binding the contract event 0x0e2fb031ee032dc02d8011dc50b816eb450cf856abd8261680dac74f72165bd2.
//
// Solidity: event Paused(bool isPaused)
func (_KlingonHTLC *KlingonHTLCFilterer) WatchPaused(opts *bind.WatchOpts, sink chan<- *KlingonHTLCPaused) (event.Subscription, error) {

	logs, sub, err := _KlingonHTLC.contract.WatchLogs(opts, "Paused")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(KlingonHTLCPaused)
				if err := _KlingonHTLC.contract.UnpackLog(event, "Paused", log); err != nil {
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

// ParsePaused is a log parse operation binding the contract event 0x0e2fb031ee032dc02d8011dc50b816eb450cf856abd8261680dac74f72165bd2.
//
// Solidity: event Paused(bool isPaused)
func (_KlingonHTLC *KlingonHTLCFilterer) ParsePaused(log types.Log) (*KlingonHTLCPaused, error) {
	event := new(KlingonHTLCPaused)
	if err := _KlingonHTLC.contract.UnpackLog(event, "Paused", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// KlingonHTLCSwapClaimedIterator is returned from FilterSwapClaimed and is used to iterate over the raw logs and unpacked data for SwapClaimed events raised by the KlingonHTLC contract.
type KlingonHTLCSwapClaimedIterator struct {
	Event *KlingonHTLCSwapClaimed // Event containing the contract specifics and raw log

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
func (it *KlingonHTLCSwapClaimedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(KlingonHTLCSwapClaimed)
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
		it.Event = new(KlingonHTLCSwapClaimed)
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
func (it *KlingonHTLCSwapClaimedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *KlingonHTLCSwapClaimedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// KlingonHTLCSwapClaimed represents a SwapClaimed event raised by the KlingonHTLC contract.
type KlingonHTLCSwapClaimed struct {
	SwapId   [32]byte
	Receiver common.Address
	Secret   [32]byte
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterSwapClaimed is a free log retrieval operation binding the contract event 0xec362e2905f5025b0b88fa68647b50bdcadb528aac989973cb6ab041177bd782.
//
// Solidity: event SwapClaimed(bytes32 indexed swapId, address indexed receiver, bytes32 secret)
func (_KlingonHTLC *KlingonHTLCFilterer) FilterSwapClaimed(opts *bind.FilterOpts, swapId [][32]byte, receiver []common.Address) (*KlingonHTLCSwapClaimedIterator, error) {

	var swapIdRule []interface{}
	for _, swapIdItem := range swapId {
		swapIdRule = append(swapIdRule, swapIdItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _KlingonHTLC.contract.FilterLogs(opts, "SwapClaimed", swapIdRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCSwapClaimedIterator{contract: _KlingonHTLC.contract, event: "SwapClaimed", logs: logs, sub: sub}, nil
}

// WatchSwapClaimed is a free log subscription operation binding the contract event 0xec362e2905f5025b0b88fa68647b50bdcadb528aac989973cb6ab041177bd782.
//
// Solidity: event SwapClaimed(bytes32 indexed swapId, address indexed receiver, bytes32 secret)
func (_KlingonHTLC *KlingonHTLCFilterer) WatchSwapClaimed(opts *bind.WatchOpts, sink chan<- *KlingonHTLCSwapClaimed, swapId [][32]byte, receiver []common.Address) (event.Subscription, error) {

	var swapIdRule []interface{}
	for _, swapIdItem := range swapId {
		swapIdRule = append(swapIdRule, swapIdItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _KlingonHTLC.contract.WatchLogs(opts, "SwapClaimed", swapIdRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(KlingonHTLCSwapClaimed)
				if err := _KlingonHTLC.contract.UnpackLog(event, "SwapClaimed", log); err != nil {
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

// ParseSwapClaimed is a log parse operation binding the contract event 0xec362e2905f5025b0b88fa68647b50bdcadb528aac989973cb6ab041177bd782.
//
// Solidity: event SwapClaimed(bytes32 indexed swapId, address indexed receiver, bytes32 secret)
func (_KlingonHTLC *KlingonHTLCFilterer) ParseSwapClaimed(log types.Log) (*KlingonHTLCSwapClaimed, error) {
	event := new(KlingonHTLCSwapClaimed)
	if err := _KlingonHTLC.contract.UnpackLog(event, "SwapClaimed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// KlingonHTLCSwapCreatedIterator is returned from FilterSwapCreated and is used to iterate over the raw logs and unpacked data for SwapCreated events raised by the KlingonHTLC contract.
type KlingonHTLCSwapCreatedIterator struct {
	Event *KlingonHTLCSwapCreated // Event containing the contract specifics and raw log

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
func (it *KlingonHTLCSwapCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(KlingonHTLCSwapCreated)
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
		it.Event = new(KlingonHTLCSwapCreated)
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
func (it *KlingonHTLCSwapCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *KlingonHTLCSwapCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// KlingonHTLCSwapCreated represents a SwapCreated event raised by the KlingonHTLC contract.
type KlingonHTLCSwapCreated struct {
	SwapId     [32]byte
	Sender     common.Address
	Receiver   common.Address
	Token      common.Address
	Amount     *big.Int
	DaoFee     *big.Int
	SecretHash [32]byte
	Timelock   *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterSwapCreated is a free log retrieval operation binding the contract event 0xe584d5707a073b24a82c4414bac2dd9c326e3a1eaeb0b74ca6c885c4a20de0fc.
//
// Solidity: event SwapCreated(bytes32 indexed swapId, address indexed sender, address indexed receiver, address token, uint256 amount, uint256 daoFee, bytes32 secretHash, uint256 timelock)
func (_KlingonHTLC *KlingonHTLCFilterer) FilterSwapCreated(opts *bind.FilterOpts, swapId [][32]byte, sender []common.Address, receiver []common.Address) (*KlingonHTLCSwapCreatedIterator, error) {

	var swapIdRule []interface{}
	for _, swapIdItem := range swapId {
		swapIdRule = append(swapIdRule, swapIdItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _KlingonHTLC.contract.FilterLogs(opts, "SwapCreated", swapIdRule, senderRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCSwapCreatedIterator{contract: _KlingonHTLC.contract, event: "SwapCreated", logs: logs, sub: sub}, nil
}

// WatchSwapCreated is a free log subscription operation binding the contract event 0xe584d5707a073b24a82c4414bac2dd9c326e3a1eaeb0b74ca6c885c4a20de0fc.
//
// Solidity: event SwapCreated(bytes32 indexed swapId, address indexed sender, address indexed receiver, address token, uint256 amount, uint256 daoFee, bytes32 secretHash, uint256 timelock)
func (_KlingonHTLC *KlingonHTLCFilterer) WatchSwapCreated(opts *bind.WatchOpts, sink chan<- *KlingonHTLCSwapCreated, swapId [][32]byte, sender []common.Address, receiver []common.Address) (event.Subscription, error) {

	var swapIdRule []interface{}
	for _, swapIdItem := range swapId {
		swapIdRule = append(swapIdRule, swapIdItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}
	var receiverRule []interface{}
	for _, receiverItem := range receiver {
		receiverRule = append(receiverRule, receiverItem)
	}

	logs, sub, err := _KlingonHTLC.contract.WatchLogs(opts, "SwapCreated", swapIdRule, senderRule, receiverRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(KlingonHTLCSwapCreated)
				if err := _KlingonHTLC.contract.UnpackLog(event, "SwapCreated", log); err != nil {
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

// ParseSwapCreated is a log parse operation binding the contract event 0xe584d5707a073b24a82c4414bac2dd9c326e3a1eaeb0b74ca6c885c4a20de0fc.
//
// Solidity: event SwapCreated(bytes32 indexed swapId, address indexed sender, address indexed receiver, address token, uint256 amount, uint256 daoFee, bytes32 secretHash, uint256 timelock)
func (_KlingonHTLC *KlingonHTLCFilterer) ParseSwapCreated(log types.Log) (*KlingonHTLCSwapCreated, error) {
	event := new(KlingonHTLCSwapCreated)
	if err := _KlingonHTLC.contract.UnpackLog(event, "SwapCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// KlingonHTLCSwapRefundedIterator is returned from FilterSwapRefunded and is used to iterate over the raw logs and unpacked data for SwapRefunded events raised by the KlingonHTLC contract.
type KlingonHTLCSwapRefundedIterator struct {
	Event *KlingonHTLCSwapRefunded // Event containing the contract specifics and raw log

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
func (it *KlingonHTLCSwapRefundedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(KlingonHTLCSwapRefunded)
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
		it.Event = new(KlingonHTLCSwapRefunded)
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
func (it *KlingonHTLCSwapRefundedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *KlingonHTLCSwapRefundedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// KlingonHTLCSwapRefunded represents a SwapRefunded event raised by the KlingonHTLC contract.
type KlingonHTLCSwapRefunded struct {
	SwapId [32]byte
	Sender common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterSwapRefunded is a free log retrieval operation binding the contract event 0xc672feaa452bd52b0000f3d29c943cd9331556ab05529d49e984311220c16c19.
//
// Solidity: event SwapRefunded(bytes32 indexed swapId, address indexed sender)
func (_KlingonHTLC *KlingonHTLCFilterer) FilterSwapRefunded(opts *bind.FilterOpts, swapId [][32]byte, sender []common.Address) (*KlingonHTLCSwapRefundedIterator, error) {

	var swapIdRule []interface{}
	for _, swapIdItem := range swapId {
		swapIdRule = append(swapIdRule, swapIdItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _KlingonHTLC.contract.FilterLogs(opts, "SwapRefunded", swapIdRule, senderRule)
	if err != nil {
		return nil, err
	}
	return &KlingonHTLCSwapRefundedIterator{contract: _KlingonHTLC.contract, event: "SwapRefunded", logs: logs, sub: sub}, nil
}

// WatchSwapRefunded is a free log subscription operation binding the contract event 0xc672feaa452bd52b0000f3d29c943cd9331556ab05529d49e984311220c16c19.
//
// Solidity: event SwapRefunded(bytes32 indexed swapId, address indexed sender)
func (_KlingonHTLC *KlingonHTLCFilterer) WatchSwapRefunded(opts *bind.WatchOpts, sink chan<- *KlingonHTLCSwapRefunded, swapId [][32]byte, sender []common.Address) (event.Subscription, error) {

	var swapIdRule []interface{}
	for _, swapIdItem := range swapId {
		swapIdRule = append(swapIdRule, swapIdItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _KlingonHTLC.contract.WatchLogs(opts, "SwapRefunded", swapIdRule, senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(KlingonHTLCSwapRefunded)
				if err := _KlingonHTLC.contract.UnpackLog(event, "SwapRefunded", log); err != nil {
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

// ParseSwapRefunded is a log parse operation binding the contract event 0xc672feaa452bd52b0000f3d29c943cd9331556ab05529d49e984311220c16c19.
//
// Solidity: event SwapRefunded(bytes32 indexed swapId, address indexed sender)
func (_KlingonHTLC *KlingonHTLCFilterer) ParseSwapRefunded(log types.Log) (*KlingonHTLCSwapRefunded, error) {
	event := new(KlingonHTLCSwapRefunded)
	if err := _KlingonHTLC.contract.UnpackLog(event, "SwapRefunded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
