// Package htlc provides a Go client for interacting with the KlingonHTLC smart contract.
// This client wraps the auto-generated bindings with a more user-friendly interface.
package htlc

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// SwapState represents the state of an HTLC swap
type SwapState uint8

const (
	SwapStateEmpty    SwapState = 0
	SwapStateActive   SwapState = 1
	SwapStateClaimed  SwapState = 2
	SwapStateRefunded SwapState = 3
)

func (s SwapState) String() string {
	switch s {
	case SwapStateEmpty:
		return "empty"
	case SwapStateActive:
		return "active"
	case SwapStateClaimed:
		return "claimed"
	case SwapStateRefunded:
		return "refunded"
	default:
		return "unknown"
	}
}

// Swap represents an HTLC swap with parsed fields
type Swap struct {
	Sender     common.Address
	Receiver   common.Address
	Token      common.Address // address(0) for native token
	Amount     *big.Int
	DaoFee     *big.Int
	SecretHash [32]byte
	Timelock   *big.Int
	State      SwapState
}

// IsNativeToken returns true if this swap uses native token (ETH/BNB)
func (s *Swap) IsNativeToken() bool {
	return s.Token == common.Address{}
}

// IsActive returns true if the swap is active
func (s *Swap) IsActive() bool {
	return s.State == SwapStateActive
}

// Client is a wrapper around the KlingonHTLC contract
type Client struct {
	client          *ethclient.Client
	contract        *KlingonHTLC
	contractAddress common.Address
	chainID         *big.Int
}

// NewClient creates a new HTLC client
func NewClient(rpcURL string, contractAddress common.Address) (*Client, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RPC: %w", err)
	}

	contract, err := NewKlingonHTLC(contractAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to bind contract: %w", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %w", err)
	}

	return &Client{
		client:          client,
		contract:        contract,
		contractAddress: contractAddress,
		chainID:         chainID,
	}, nil
}

// Close closes the underlying RPC connection
func (c *Client) Close() {
	c.client.Close()
}

// ChainID returns the chain ID
func (c *Client) ChainID() *big.Int {
	return c.chainID
}

// ContractAddress returns the contract address
func (c *Client) ContractAddress() common.Address {
	return c.contractAddress
}

// =============================================================================
// Secret Generation
// =============================================================================

// GenerateSecret creates a new 32-byte secret and its SHA256 hash
func GenerateSecret() (secret [32]byte, hash [32]byte, err error) {
	_, err = rand.Read(secret[:])
	if err != nil {
		return [32]byte{}, [32]byte{}, fmt.Errorf("failed to generate random secret: %w", err)
	}
	hash = sha256.Sum256(secret[:])
	return secret, hash, nil
}

// HashSecret computes the SHA256 hash of a secret
func HashSecret(secret [32]byte) [32]byte {
	return sha256.Sum256(secret[:])
}

// VerifySecret checks if a secret matches a hash
func VerifySecret(secret, hash [32]byte) bool {
	computed := sha256.Sum256(secret[:])
	return computed == hash
}

// =============================================================================
// Swap ID Computation
// =============================================================================

// ComputeSwapID computes a deterministic swap ID from parameters
func (c *Client) ComputeSwapID(
	ctx context.Context,
	sender, receiver, token common.Address,
	amount *big.Int,
	secretHash [32]byte,
	timelock *big.Int,
	nonce *big.Int,
) ([32]byte, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.ComputeSwapId(opts, sender, receiver, token, amount, secretHash, timelock, nonce)
}

// =============================================================================
// Swap Creation
// =============================================================================

// CreateSwapNative creates a swap with native token (ETH/BNB/etc)
func (c *Client) CreateSwapNative(
	ctx context.Context,
	privateKey *ecdsa.PrivateKey,
	swapID [32]byte,
	receiver common.Address,
	secretHash [32]byte,
	timelock *big.Int,
	amount *big.Int,
) (*types.Transaction, error) {
	auth, err := c.newTransactor(ctx, privateKey)
	if err != nil {
		return nil, err
	}
	auth.Value = amount

	return c.contract.CreateSwapNative(auth, swapID, receiver, secretHash, timelock)
}

// CreateSwapERC20 creates a swap with an ERC20 token
// Note: Token must be approved first using ApproveERC20
func (c *Client) CreateSwapERC20(
	ctx context.Context,
	privateKey *ecdsa.PrivateKey,
	swapID [32]byte,
	receiver common.Address,
	token common.Address,
	amount *big.Int,
	secretHash [32]byte,
	timelock *big.Int,
) (*types.Transaction, error) {
	auth, err := c.newTransactor(ctx, privateKey)
	if err != nil {
		return nil, err
	}

	return c.contract.CreateSwapERC20(auth, swapID, receiver, token, amount, secretHash, timelock)
}

// =============================================================================
// Claim and Refund
// =============================================================================

// Claim claims a swap by revealing the secret
func (c *Client) Claim(
	ctx context.Context,
	privateKey *ecdsa.PrivateKey,
	swapID [32]byte,
	secret [32]byte,
) (*types.Transaction, error) {
	auth, err := c.newTransactor(ctx, privateKey)
	if err != nil {
		return nil, err
	}

	return c.contract.Claim(auth, swapID, secret)
}

// Refund refunds a swap after the timelock expires
func (c *Client) Refund(
	ctx context.Context,
	privateKey *ecdsa.PrivateKey,
	swapID [32]byte,
) (*types.Transaction, error) {
	auth, err := c.newTransactor(ctx, privateKey)
	if err != nil {
		return nil, err
	}

	return c.contract.Refund(auth, swapID)
}

// =============================================================================
// View Functions
// =============================================================================

// GetSwap returns the swap details
func (c *Client) GetSwap(ctx context.Context, swapID [32]byte) (*Swap, error) {
	opts := &bind.CallOpts{Context: ctx}
	result, err := c.contract.GetSwap(opts, swapID)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap: %w", err)
	}

	return &Swap{
		Sender:     result.Sender,
		Receiver:   result.Receiver,
		Token:      result.Token,
		Amount:     result.Amount,
		DaoFee:     result.DaoFee,
		SecretHash: result.SecretHash,
		Timelock:   result.Timelock,
		State:      SwapState(result.State),
	}, nil
}

// CanClaim returns true if the swap can be claimed
func (c *Client) CanClaim(ctx context.Context, swapID [32]byte) (bool, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.CanClaim(opts, swapID)
}

// CanRefund returns true if the swap can be refunded
func (c *Client) CanRefund(ctx context.Context, swapID [32]byte) (bool, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.CanRefund(opts, swapID)
}

// VerifySecretOnChain verifies a secret against the stored hash on-chain
func (c *Client) VerifySecretOnChain(ctx context.Context, swapID [32]byte, secret [32]byte) (bool, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.VerifySecret(opts, swapID, secret)
}

// TimeUntilRefund returns seconds until refund is possible
func (c *Client) TimeUntilRefund(ctx context.Context, swapID [32]byte) (*big.Int, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.TimeUntilRefund(opts, swapID)
}

// GetDaoAddress returns the DAO address
func (c *Client) GetDaoAddress(ctx context.Context) (common.Address, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.DaoAddress(opts)
}

// GetFeeBps returns the fee in basis points
func (c *Client) GetFeeBps(ctx context.Context) (*big.Int, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.FeeBps(opts)
}

// IsPaused returns true if the contract is paused
func (c *Client) IsPaused(ctx context.Context) (bool, error) {
	opts := &bind.CallOpts{Context: ctx}
	return c.contract.Paused(opts)
}

// =============================================================================
// Event Watching
// =============================================================================

// SwapCreatedEvent represents a SwapCreated event
type SwapCreatedEvent struct {
	SwapID     [32]byte
	Sender     common.Address
	Receiver   common.Address
	Token      common.Address
	Amount     *big.Int
	DaoFee     *big.Int
	SecretHash [32]byte
	Timelock   *big.Int
	TxHash     common.Hash
	BlockNum   uint64
}

// SwapClaimedEvent represents a SwapClaimed event (contains the revealed secret!)
type SwapClaimedEvent struct {
	SwapID   [32]byte
	Receiver common.Address
	Secret   [32]byte // The revealed secret!
	TxHash   common.Hash
	BlockNum uint64
}

// SwapRefundedEvent represents a SwapRefunded event
type SwapRefundedEvent struct {
	SwapID   [32]byte
	Sender   common.Address
	TxHash   common.Hash
	BlockNum uint64
}

// WatchSwapCreated watches for SwapCreated events
func (c *Client) WatchSwapCreated(
	ctx context.Context,
	swapIDs [][32]byte,
	senders []common.Address,
) (<-chan *SwapCreatedEvent, error) {
	// Use the generated type for the contract watcher
	ch := make(chan *KlingonHTLCSwapCreated, 10)

	sub, err := c.contract.WatchSwapCreated(
		&bind.WatchOpts{Context: ctx},
		ch,
		swapIDs,
		senders,
		nil, // receivers
	)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to watch SwapCreated: %w", err)
	}

	// Create output channel with parsed events
	outCh := make(chan *SwapCreatedEvent, 10)
	go func() {
		defer close(outCh)
		defer sub.Unsubscribe()

		for {
			select {
			case event := <-ch:
				if event == nil {
					return
				}
				outCh <- &SwapCreatedEvent{
					SwapID:     event.SwapId,
					Sender:     event.Sender,
					Receiver:   event.Receiver,
					Token:      event.Token,
					Amount:     event.Amount,
					DaoFee:     event.DaoFee,
					SecretHash: event.SecretHash,
					Timelock:   event.Timelock,
					TxHash:     event.Raw.TxHash,
					BlockNum:   event.Raw.BlockNumber,
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

// WatchSwapClaimed watches for SwapClaimed events
// This is critical for cross-chain swaps as it reveals the secret!
func (c *Client) WatchSwapClaimed(
	ctx context.Context,
	swapIDs [][32]byte,
) (<-chan *SwapClaimedEvent, error) {
	ch := make(chan *KlingonHTLCSwapClaimed, 10)

	sub, err := c.contract.WatchSwapClaimed(
		&bind.WatchOpts{Context: ctx},
		ch,
		swapIDs,
		nil, // receivers
	)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to watch SwapClaimed: %w", err)
	}

	// Create output channel with parsed events
	outCh := make(chan *SwapClaimedEvent, 10)
	go func() {
		defer close(outCh)
		defer sub.Unsubscribe()

		for {
			select {
			case event := <-ch:
				outCh <- &SwapClaimedEvent{
					SwapID:   event.SwapId,
					Receiver: event.Receiver,
					Secret:   event.Secret,
					TxHash:   event.Raw.TxHash,
					BlockNum: event.Raw.BlockNumber,
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

// WatchSwapRefunded watches for SwapRefunded events
func (c *Client) WatchSwapRefunded(
	ctx context.Context,
	swapIDs [][32]byte,
) (<-chan *SwapRefundedEvent, error) {
	ch := make(chan *KlingonHTLCSwapRefunded, 10)

	sub, err := c.contract.WatchSwapRefunded(
		&bind.WatchOpts{Context: ctx},
		ch,
		swapIDs,
		nil, // senders
	)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to watch SwapRefunded: %w", err)
	}

	// Create output channel with parsed events
	outCh := make(chan *SwapRefundedEvent, 10)
	go func() {
		defer close(outCh)
		defer sub.Unsubscribe()

		for {
			select {
			case event := <-ch:
				outCh <- &SwapRefundedEvent{
					SwapID:   event.SwapId,
					Sender:   event.Sender,
					TxHash:   event.Raw.TxHash,
					BlockNum: event.Raw.BlockNumber,
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

// WaitForSecret waits for a swap to be claimed and returns the secret
func (c *Client) WaitForSecret(ctx context.Context, swapID [32]byte) ([32]byte, error) {
	ch, err := c.WatchSwapClaimed(ctx, [][32]byte{swapID})
	if err != nil {
		return [32]byte{}, err
	}

	select {
	case event := <-ch:
		if event == nil {
			return [32]byte{}, fmt.Errorf("channel closed without event")
		}
		return event.Secret, nil
	case <-ctx.Done():
		return [32]byte{}, ctx.Err()
	}
}

// =============================================================================
// Historical Event Queries
// =============================================================================

// GetSwapCreatedEvents queries historical SwapCreated events
func (c *Client) GetSwapCreatedEvents(
	ctx context.Context,
	fromBlock, toBlock uint64,
	swapIDs [][32]byte,
) ([]*SwapCreatedEvent, error) {
	opts := &bind.FilterOpts{
		Start:   fromBlock,
		End:     &toBlock,
		Context: ctx,
	}

	iter, err := c.contract.FilterSwapCreated(opts, swapIDs, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to filter SwapCreated: %w", err)
	}
	defer iter.Close()

	var events []*SwapCreatedEvent
	for iter.Next() {
		event := iter.Event
		events = append(events, &SwapCreatedEvent{
			SwapID:     event.SwapId,
			Sender:     event.Sender,
			Receiver:   event.Receiver,
			Token:      event.Token,
			Amount:     event.Amount,
			DaoFee:     event.DaoFee,
			SecretHash: event.SecretHash,
			Timelock:   event.Timelock,
			TxHash:     event.Raw.TxHash,
			BlockNum:   event.Raw.BlockNumber,
		})
	}

	return events, nil
}

// GetSwapClaimedEvents queries historical SwapClaimed events
func (c *Client) GetSwapClaimedEvents(
	ctx context.Context,
	fromBlock, toBlock uint64,
	swapIDs [][32]byte,
) ([]*SwapClaimedEvent, error) {
	opts := &bind.FilterOpts{
		Start:   fromBlock,
		End:     &toBlock,
		Context: ctx,
	}

	iter, err := c.contract.FilterSwapClaimed(opts, swapIDs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to filter SwapClaimed: %w", err)
	}
	defer iter.Close()

	var events []*SwapClaimedEvent
	for iter.Next() {
		event := iter.Event
		events = append(events, &SwapClaimedEvent{
			SwapID:   event.SwapId,
			Receiver: event.Receiver,
			Secret:   event.Secret,
			TxHash:   event.Raw.TxHash,
			BlockNum: event.Raw.BlockNumber,
		})
	}

	return events, nil
}

// GetSecretFromClaim extracts the secret from a claim transaction
// Useful when you know a claim happened but missed the event
func (c *Client) GetSecretFromClaim(ctx context.Context, txHash common.Hash) ([32]byte, error) {
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to get receipt: %w", err)
	}

	// Parse logs for SwapClaimed event
	for _, log := range receipt.Logs {
		if log.Address != c.contractAddress {
			continue
		}

		event, err := c.contract.ParseSwapClaimed(*log)
		if err != nil {
			continue // Not a SwapClaimed event
		}

		return event.Secret, nil
	}

	return [32]byte{}, fmt.Errorf("no SwapClaimed event found in transaction")
}

// =============================================================================
// Transaction Helpers
// =============================================================================

// WaitForTx waits for a transaction to be mined and returns the receipt
func (c *Client) WaitForTx(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {
	return bind.WaitMined(ctx, c.client, tx)
}

// WaitForTxWithTimeout waits for a transaction with a timeout
func (c *Client) WaitForTxWithTimeout(tx *types.Transaction, timeout time.Duration) (*types.Receipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.WaitForTx(ctx, tx)
}

// EstimateGasCreateSwapNative estimates gas for creating a native swap
func (c *Client) EstimateGasCreateSwapNative(
	ctx context.Context,
	from common.Address,
	swapID [32]byte,
	receiver common.Address,
	secretHash [32]byte,
	timelock *big.Int,
	amount *big.Int,
) (uint64, error) {
	// Pack the function call
	abi, err := KlingonHTLCMetaData.GetAbi()
	if err != nil {
		return 0, err
	}

	data, err := abi.Pack("createSwapNative", swapID, receiver, secretHash, timelock)
	if err != nil {
		return 0, err
	}

	msg := ethereum.CallMsg{
		From:  from,
		To:    &c.contractAddress,
		Value: amount,
		Data:  data,
	}

	return c.client.EstimateGas(ctx, msg)
}

// =============================================================================
// ERC20 Helpers
// =============================================================================

// ApproveERC20 approves the HTLC contract to spend tokens
func (c *Client) ApproveERC20(
	ctx context.Context,
	privateKey *ecdsa.PrivateKey,
	token common.Address,
	amount *big.Int,
) (*types.Transaction, error) {
	from := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Create approve call data
	// Function selector for approve(address,uint256) = 0x095ea7b3
	data := make([]byte, 68)
	copy(data[0:4], []byte{0x09, 0x5e, 0xa7, 0xb3})
	copy(data[4:36], common.LeftPadBytes(c.contractAddress.Bytes(), 32))
	copy(data[36:68], common.LeftPadBytes(amount.Bytes(), 32))

	// Get nonce
	nonce, err := c.client.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}

	// Get gas price
	gasPrice, err := c.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	// Create transaction
	tx := types.NewTransaction(nonce, token, big.NewInt(0), 60000, gasPrice, data)

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(c.chainID), privateKey)
	if err != nil {
		return nil, err
	}

	// Send transaction
	err = c.client.SendTransaction(ctx, signedTx)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

// =============================================================================
// Internal Helpers
// =============================================================================

func (c *Client) newTransactor(ctx context.Context, privateKey *ecdsa.PrivateKey) (*bind.TransactOpts, error) {
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, c.chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %w", err)
	}
	auth.Context = ctx
	return auth, nil
}

// AddressFromPrivateKey derives the address from a private key
func AddressFromPrivateKey(privateKey *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(privateKey.PublicKey)
}

// ParsePrivateKey parses a hex-encoded private key
func ParsePrivateKey(hexKey string) (*ecdsa.PrivateKey, error) {
	return crypto.HexToECDSA(hexKey)
}
