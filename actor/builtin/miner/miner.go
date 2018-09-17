package miner

import (
	"fmt"
	"math/big"

	"gx/ipfs/QmQsErDt8Qgw1XrsXf2BpEzDgGWtB1YLsTAARBup5b6B9W/go-libp2p-peer"
	cbor "gx/ipfs/QmV6BQ6fFCf9eFHDuRxvguvqfKLZtZrxthgZvDfRCs4tMN/go-ipld-cbor"
	xerrors "gx/ipfs/QmVmDhyTTUcQXFD1rRQ64fGLMSAoaQvNH3hwuaCFAPq2hy/errors"

	"github.com/filecoin-project/go-filecoin/abi"
	"github.com/filecoin-project/go-filecoin/actor"
	"github.com/filecoin-project/go-filecoin/address"
	"github.com/filecoin-project/go-filecoin/exec"
	"github.com/filecoin-project/go-filecoin/types"
	"github.com/filecoin-project/go-filecoin/vm/errors"
)

func init() {
	cbor.RegisterCborType(State{})
}

// MaximumPublicKeySize is a limit on how big a public key can be.
const MaximumPublicKeySize = 100

// ProvingPeriodBlocks defines how long a proving period is for
var ProvingPeriodBlocks = types.NewBlockHeight(2000)

const (
	// ErrPublicKeyTooBig indicates an invalid public key.
	ErrPublicKeyTooBig = 33
	// ErrInvalidSector indicates and invalid sector id.
	ErrInvalidSector = 34
	// ErrSectorCommitted indicates the sector has already been committed.
	ErrSectorCommitted = 35
	// ErrStoragemarketCallFailed indicates the call to commit the deal failed.
	ErrStoragemarketCallFailed = 36
	// ErrCallerUnauthorized signals an unauthorized caller.
	ErrCallerUnauthorized = 37
	// ErrInsufficientPledge signals insufficient pledge for what you are trying to do.
	ErrInsufficientPledge = 38
)

// Errors map error codes to revert errors this actor may return.
var Errors = map[uint8]error{
	ErrPublicKeyTooBig:         errors.NewCodedRevertErrorf(ErrPublicKeyTooBig, "public key must be less than %d bytes", MaximumPublicKeySize),
	ErrInvalidSector:           errors.NewCodedRevertErrorf(ErrInvalidSector, "sectorID out of range"),
	ErrSectorCommitted:         errors.NewCodedRevertErrorf(ErrSectorCommitted, "sector already committed"),
	ErrStoragemarketCallFailed: errors.NewCodedRevertErrorf(ErrStoragemarketCallFailed, "call to StorageMarket failed"),
	ErrCallerUnauthorized:      errors.NewCodedRevertErrorf(ErrCallerUnauthorized, "not authorized to call the method"),
	ErrInsufficientPledge:      errors.NewCodedRevertErrorf(ErrInsufficientPledge, "not enough pledged"),
}

// Actor is the miner actor.
type Actor struct{}

// State is the miner actors storage.
type State struct {
	Owner address.Address

	// PeerID references the libp2p identity that the miner is operating.
	PeerID peer.ID

	// PublicKey is used to validate blocks generated by the miner this actor represents.
	PublicKey []byte

	// Pledge is amount the space being offered up by this miner.
	// TODO: maybe minimum granularity is more than 1 byte?
	PledgeBytes *types.BytesAmount

	// Collateral is the total amount of filecoin being held as collateral for
	// the miners pledge.
	Collateral *types.AttoFIL

	// Sectors maps commR to commD, for all sectors this miner has committed.
	Sectors map[string][]byte

	ProvingPeriodStart *types.BlockHeight
	LastPoSt           *types.BlockHeight

	LockedStorage *types.BytesAmount // LockedStorage is the amount of the miner's storage that is used.
	Power         *big.Int
}

// NewActor returns a new miner actor
func NewActor() *actor.Actor {
	return actor.NewActor(types.MinerActorCodeCid, types.NewZeroAttoFIL())
}

// NewState creates a miner state struct
func NewState(owner address.Address, key []byte, pledge *types.BytesAmount, pid peer.ID, collateral *types.AttoFIL) *State {
	return &State{
		Owner:         owner,
		PeerID:        pid,
		PublicKey:     key,
		PledgeBytes:   pledge,
		Collateral:    collateral,
		LockedStorage: types.NewBytesAmount(0),
		Sectors:       make(map[string][]byte),
		Power:         big.NewInt(0),
	}
}

// InitializeState stores this miner's initial data structure.
func (ma *Actor) InitializeState(storage exec.Storage, initializerData interface{}) error {
	minerState, ok := initializerData.(*State)
	if !ok {
		return errors.NewFaultError("Initial state to miner actor is not a miner.State struct")
	}

	// TODO: we should validate this is actually a public key (possibly the owner's public key) once we have a better
	// TODO: idea what crypto looks like.
	if len(minerState.PublicKey) > MaximumPublicKeySize {
		return Errors[ErrPublicKeyTooBig]
	}

	stateBytes, err := cbor.DumpObject(minerState)
	if err != nil {
		return xerrors.Wrap(err, "failed to cbor marshal object")
	}

	id, err := storage.Put(stateBytes)
	if err != nil {
		return err
	}

	return storage.Commit(id, nil)
}

var _ exec.ExecutableActor = (*Actor)(nil)

var minerExports = exec.Exports{
	"addAsk": &exec.FunctionSignature{
		Params: []abi.Type{abi.AttoFIL, abi.BytesAmount},
		Return: []abi.Type{abi.Integer},
	},
	"getOwner": &exec.FunctionSignature{
		Params: nil,
		Return: []abi.Type{abi.Address},
	},
	"commitSector": &exec.FunctionSignature{
		Params: []abi.Type{abi.SectorID, abi.Bytes, abi.Bytes},
		Return: []abi.Type{},
	},
	"getKey": &exec.FunctionSignature{
		Params: []abi.Type{},
		Return: []abi.Type{abi.Bytes},
	},
	"getPeerID": &exec.FunctionSignature{
		Params: []abi.Type{},
		Return: []abi.Type{abi.PeerID},
	},
	"updatePeerID": &exec.FunctionSignature{
		Params: []abi.Type{abi.PeerID},
		Return: []abi.Type{},
	},
	"getStorage": &exec.FunctionSignature{
		Params: []abi.Type{},
		Return: []abi.Type{abi.BytesAmount},
	},
	"submitPoSt": &exec.FunctionSignature{
		Params: []abi.Type{abi.Bytes},
		Return: []abi.Type{},
	},
	"getProvingPeriodStart": &exec.FunctionSignature{
		Params: []abi.Type{},
		Return: []abi.Type{abi.BlockHeight},
	},
}

// Exports returns the miner actors exported functions.
func (ma *Actor) Exports() exec.Exports {
	return minerExports
}

// AddAsk adds an ask via this miner to the storage markets orderbook.
func (ma *Actor) AddAsk(ctx exec.VMContext, price *types.AttoFIL, size *types.BytesAmount) (*big.Int, uint8,
	error) {
	var state State
	out, err := actor.WithState(ctx, &state, func() (interface{}, error) {
		if ctx.Message().From != state.Owner {
			return nil, Errors[ErrCallerUnauthorized]
		}

		// compute locked storage + new ask
		total := state.LockedStorage.Add(size)

		if total.GreaterThan(state.PledgeBytes) {
			return nil, Errors[ErrInsufficientPledge]
		}

		state.LockedStorage = total

		// TODO: kinda feels weird that I can't get a real type back here
		out, ret, err := ctx.Send(address.StorageMarketAddress, "addAsk", nil, []interface{}{price, size})
		if err != nil {
			return nil, err
		}

		askID, err := abi.Deserialize(out[0], abi.Integer)
		if err != nil {
			return nil, errors.FaultErrorWrap(err, "error deserializing")
		}

		if ret != 0 {
			return nil, Errors[ErrStoragemarketCallFailed]
		}

		return askID.Val, nil
	})
	if err != nil {
		return nil, errors.CodeError(err), err
	}

	askID, ok := out.(*big.Int)
	if !ok {
		return nil, 1, errors.NewRevertErrorf("expected an Integer return value from call, but got %T instead", out)
	}

	return askID, 0, nil
}

// GetOwner returns the miners owner.
func (ma *Actor) GetOwner(ctx exec.VMContext) (address.Address, uint8, error) {
	var state State
	out, err := actor.WithState(ctx, &state, func() (interface{}, error) {
		return state.Owner, nil
	})
	if err != nil {
		return address.Address{}, errors.CodeError(err), err
	}

	a, ok := out.(address.Address)
	if !ok {
		return address.Address{}, 1, errors.NewFaultErrorf("expected an Address return value from call, but got %T instead", out)
	}

	return a, 0, nil
}

// CommitSector adds a commitment to the specified sector
// The sector must not already be committed
// 'size' is the total number of bytes stored in the sector
func (ma *Actor) CommitSector(ctx exec.VMContext, sectorID uint64, commR, commD []byte) (uint8, error) {
	var state State
	_, err := actor.WithState(ctx, &state, func() (interface{}, error) {
		commRstr := string(commR) // proper fixed length array encoding in cbor is apparently 'hard'.
		_, ok := state.Sectors[commRstr]
		if ok {
			return nil, Errors[ErrSectorCommitted]
		}

		if state.Power.Cmp(big.NewInt(0)) == 0 {
			fmt.Println("starting proving period", ctx.BlockHeight())
			state.ProvingPeriodStart = ctx.BlockHeight()
		}
		inc := big.NewInt(1)
		state.Power = state.Power.Add(state.Power, inc)
		state.Sectors[commRstr] = commD

		_, ret, err := ctx.Send(address.StorageMarketAddress, "updatePower", nil, []interface{}{inc})
		if err != nil {
			return nil, err
		}
		if ret != 0 {
			return nil, Errors[ErrStoragemarketCallFailed]
		}
		return nil, nil
	})
	if err != nil {
		return errors.CodeError(err), err
	}

	return 0, nil
}

// GetKey returns the public key for this miner.
func (ma *Actor) GetKey(ctx exec.VMContext) ([]byte, uint8, error) {
	var state State
	out, err := actor.WithState(ctx, &state, func() (interface{}, error) {
		return state.PublicKey, nil
	})
	if err != nil {
		return nil, errors.CodeError(err), err
	}

	validOut, ok := out.([]byte)
	if !ok {
		return nil, 1, errors.NewRevertError("expected a byte slice")
	}

	return validOut, 0, nil
}

// GetPeerID returns the libp2p peer ID that this miner can be reached at.
func (ma *Actor) GetPeerID(ctx exec.VMContext) (peer.ID, uint8, error) {
	var state State

	chunk, err := ctx.ReadStorage()
	if err != nil {
		return peer.ID(""), errors.CodeError(err), err
	}

	if err := actor.UnmarshalStorage(chunk, &state); err != nil {
		return peer.ID(""), errors.CodeError(err), err
	}

	return state.PeerID, 0, nil
}

// UpdatePeerID is used to update the peerID this miner is operating under.
func (ma *Actor) UpdatePeerID(ctx exec.VMContext, pid peer.ID) (uint8, error) {
	var storage State
	_, err := actor.WithState(ctx, &storage, func() (interface{}, error) {
		// verify that the caller is authorized to perform update
		if ctx.Message().From != storage.Owner {
			return nil, Errors[ErrCallerUnauthorized]
		}

		storage.PeerID = pid

		return nil, nil
	})
	if err != nil {
		return errors.CodeError(err), err
	}

	return 0, nil
}

// GetStorage returns the amount of proven storage for this miner.
func (ma *Actor) GetStorage(ctx exec.VMContext) (*types.BytesAmount, uint8, error) {
	var state State
	ret, err := actor.WithState(ctx, &state, func() (interface{}, error) {
		return state.Power, nil
	})
	if err != nil {
		return nil, errors.CodeError(err), err
	}

	count, ok := ret.(*types.BytesAmount)
	if !ok {
		return nil, 1, fmt.Errorf("expected *BytesAmount to be returned, but got %T instead", ret)
	}

	return count, 0, nil
}

// SubmitPoSt is used to submit a coalesced PoST to the chain to convince the chain
// that you have been actually storing the files you claim to be.
func (ma *Actor) SubmitPoSt(ctx exec.VMContext, proof []byte) (uint8, error) {
	var state State
	_, err := actor.WithState(ctx, &state, func() (interface{}, error) {
		// verify that the caller is authorized to perform update
		fmt.Println("submitting proof", proof, ctx.Message().From, state.Owner)
		if ctx.Message().From != state.Owner {
			return nil, Errors[ErrCallerUnauthorized]
		}

		// TODO: validate the PoSt

		// Check if we submitted it in time
		provingPeriodEnd := state.ProvingPeriodStart.Add(ProvingPeriodBlocks)

		if ctx.BlockHeight().LessEqual(provingPeriodEnd) {
			state.ProvingPeriodStart = provingPeriodEnd
			state.LastPoSt = ctx.BlockHeight()
		} else {
			fmt.Println("late submission", ctx.BlockHeight(), provingPeriodEnd)
			// Not great.
			// TODO: charge penalty
			return nil, errors.NewRevertErrorf("submitted PoSt late, need to pay a fee")
		}

		return nil, nil
	})
	if err != nil {
		return errors.CodeError(err), err
	}

	return 0, nil
}

// GetProvingPeriodStart returns the current ProvingPeriodStart value.
func (ma *Actor) GetProvingPeriodStart(ctx exec.VMContext) (*types.BlockHeight, uint8, error) {
	chunk, err := ctx.ReadStorage()
	if err != nil {
		return nil, errors.CodeError(err), err
	}

	var state State
	if err := actor.UnmarshalStorage(chunk, &state); err != nil {
		return nil, errors.CodeError(err), err
	}

	return state.ProvingPeriodStart, 0, nil
}
