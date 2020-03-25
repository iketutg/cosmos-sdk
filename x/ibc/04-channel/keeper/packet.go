package keeper

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	client "github.com/cosmos/cosmos-sdk/x/ibc/02-client"
	connection "github.com/cosmos/cosmos-sdk/x/ibc/03-connection"
	"github.com/cosmos/cosmos-sdk/x/ibc/04-channel/exported"
	"github.com/cosmos/cosmos-sdk/x/ibc/04-channel/types"
	commitmentexported "github.com/cosmos/cosmos-sdk/x/ibc/23-commitment/exported"
)

// SendPacket  is called by a module in order to send an IBC packet on a channel
// end owned by the calling module to the corresponding module on the counterparty
// chain.
func (k Keeper) SendPacket(
	ctx sdk.Context,
	packet exported.PacketI,
) error {
	if err := packet.ValidateBasic(); err != nil {
		return err
	}

	channel, found := k.GetChannel(ctx, packet.GetSourcePort(), packet.GetSourceChannel())
	if !found {
		return sdkerrors.Wrap(types.ErrChannelNotFound, packet.GetSourceChannel())
	}

	if channel.GetState() == exported.CLOSED {
		return sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel is CLOSED (got %s)", channel.GetState().String(),
		)
	}

	// TODO: blocked by #5542
	// capKey, found := k.GetChannelCapability(ctx, packet.GetSourcePort(), packet.GetSourceChannel())
	// if !found {
	// 	return types.ErrChannelCapabilityNotFound
	// }

	// portCapabilityKey := sdk.NewKVStoreKey(capKey)

	// if !k.portKeeper.Authenticate(portCapabilityKey, packet.GetSourcePort()) {
	// 	return sdkerrors.Wrap(port.ErrInvalidPort, packet.GetSourcePort())
	// }

	if packet.GetDestinationPort() != channel.GetCounterParty().GetPortID() {
		return sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet destination port doesn't match the counterparty's port (%s ≠ %s)", packet.GetDestinationPort(), channel.GetCounterParty().GetPortID(),
		)
	}

	if packet.GetDestinationChannel() != channel.GetCounterParty().GetChannelID() {
		return sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet destination channel doesn't match the counterparty's channel (%s ≠ %s)", packet.GetDestinationChannel(), channel.GetCounterParty().GetChannelID(),
		)
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.GetConnectionHops()[0])
	if !found {
		return sdkerrors.Wrap(connection.ErrConnectionNotFound, channel.GetConnectionHops()[0])
	}

	// NOTE: assume UNINITIALIZED is a closed connection
	if connectionEnd.GetState() == connectionibctypes.UNINITIALIZED {
		return sdkerrors.Wrap(
			connection.ErrInvalidConnectionState,
			"connection is closed (i.e NONE)",
		)
	}

	clientState, found := k.clientKeeper.GetClientState(ctx, connectionEnd.GetClientID())
	if !found {
		return client.ErrConsensusStateNotFound
	}

	// check if packet timeouted on the receiving chain
	if clientState.GetLatestHeight() >= packet.GetTimeoutHeight() {
		return sdkerrors.Wrap(types.ErrPacketTimeout, "timeout already passed ond the receiving chain")
	}

	nextSequenceSend, found := k.GetNextSequenceSend(ctx, packet.GetSourcePort(), packet.GetSourceChannel())
	if !found {
		return types.ErrSequenceSendNotFound
	}

	if packet.GetSequence() != nextSequenceSend {
		return sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet sequence ≠ next send sequence (%d ≠ %d)", packet.GetSequence(), nextSequenceSend,
		)
	}

	nextSequenceSend++
	k.SetNextSequenceSend(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), nextSequenceSend)
	k.SetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence(), types.CommitPacket(packet.GetData()))

	// Emit Event with Packet data along with other packet information for relayer to pick up
	// and relay to other chain
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeSendPacket,
			sdk.NewAttribute(types.AttributeKeyData, string(packet.GetData().GetBytes())),
			sdk.NewAttribute(types.AttributeKeyTimeout, fmt.Sprintf("%d", packet.GetData().GetTimeoutHeight())),
			sdk.NewAttribute(types.AttributeKeySequence, fmt.Sprintf("%d", packet.GetSequence())),
			sdk.NewAttribute(types.AttributeKeySrcPort, packet.GetSourcePort()),
			sdk.NewAttribute(types.AttributeKeySrcChannel, packet.GetSourceChannel()),
			sdk.NewAttribute(types.AttributeKeyDstPort, packet.GetDestinationPort()),
			sdk.NewAttribute(types.AttributeKeyDstChannel, packet.GetDestinationChannel()),
		),
	})

	k.Logger(ctx).Info(fmt.Sprintf("packet sent %v", packet)) // TODO: use packet.String()
	return nil
}

// RecvPacket is called by a module in order to receive & process an IBC packet
// sent on the corresponding channel end on the counterparty chain.
func (k Keeper) RecvPacket(
	ctx sdk.Context,
	packet exported.PacketI,
	proof commitmentexported.Proof,
	proofHeight uint64,
) (exported.PacketI, error) {
	channel, found := k.GetChannel(ctx, packet.GetDestinationPort(), packet.GetDestinationChannel())
	if !found {
		return nil, sdkerrors.Wrap(types.ErrChannelNotFound, packet.GetDestinationChannel())
	}

	if channel.GetState() != ibctypes.OPEN {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel state is not OPEN (got %s)", channel.GetState().String(),
		)
	}

	// NOTE: RecvPacket is called by the AnteHandler which acts upon the packet.Route(),
	// so the capability authentication can be omitted here

	// packet must come from the channel's counterparty
	if packet.GetSourcePort() != channel.GetCounterParty().GetPortID() {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet source port doesn't match the counterparty's port (%s ≠ %s)", packet.GetSourcePort(), channel.GetCounterParty().GetPortID(),
		)
	}

	if packet.GetSourceChannel() != channel.GetCounterParty().GetChannelID() {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet source channel doesn't match the counterparty's channel (%s ≠ %s)", packet.GetSourceChannel(), channel.GetCounterParty().GetChannelID(),
		)
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.GetConnectionHops()[0])
	if !found {
		return nil, sdkerrors.Wrap(connection.ErrConnectionNotFound, channel.GetConnectionHops()[0])
	}

	if connectionEnd.GetState() != connectionibctypes.OPEN {
		return nil, sdkerrors.Wrapf(
			connection.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectionEnd.GetState().String(),
		)
	}

	// check if packet timeouted by comparing it with the latest height of the chain
	if uint64(ctx.BlockHeight()) >= packet.GetTimeoutHeight() {
		return nil, types.ErrPacketTimeout
	}

	if err := k.connectionKeeper.VerifyPacketCommitment(
		ctx, connectionEnd, proofHeight, proof,
		packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence(),
		types.CommitPacket(packet.GetData()),
	); err != nil {
		return nil, sdkerrors.Wrap(err, "couldn't verify counterparty packet commitment")
	}

	return packet, nil
}

// PacketExecuted writes the packet execution acknowledgement to the state,
// which will be verified by the counterparty chain using AcknowledgePacket.
// CONTRACT: each packet handler function should call WriteAcknowledgement at the end of the execution
func (k Keeper) PacketExecuted(
	ctx sdk.Context,
	packet exported.PacketI,
	acknowledgement exported.PacketAcknowledgementI,
) error {
	channel, found := k.GetChannel(ctx, packet.GetDestinationPort(), packet.GetDestinationChannel())
	if !found {
		return sdkerrors.Wrapf(types.ErrChannelNotFound, packet.GetDestinationChannel())
	}

	// sanity check
	if channel.GetState() != ibctypes.OPEN {
		return sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel state is not OPEN (got %s)", channel.GetState().String(),
		)
	}

	if acknowledgement != nil || channel.GetOrdering() == ibctypes.UNORDERED {
		k.SetPacketAcknowledgement(
			ctx, packet.GetDestinationPort(), packet.GetDestinationChannel(), packet.GetSequence(),
			types.CommitAcknowledgement(acknowledgement),
		)
	}

	if channel.GetOrdering() == ibctypes.ORDERED {
		nextSequenceRecv, found := k.GetNextSequenceRecv(ctx, packet.GetDestinationPort(), packet.GetDestinationChannel())
		if !found {
			return types.ErrSequenceReceiveNotFound
		}

		if packet.GetSequence() != nextSequenceRecv {
			return sdkerrors.Wrapf(
				types.ErrInvalidPacket,
				"packet sequence ≠ next receive sequence (%d ≠ %d)", packet.GetSequence(), nextSequenceRecv,
			)
		}

		nextSequenceRecv++

		k.SetNextSequenceRecv(ctx, packet.GetDestinationPort(), packet.GetDestinationChannel(), nextSequenceRecv)
	}

	// log that a packet has been received & acknowledged
	k.Logger(ctx).Info(fmt.Sprintf("packet received %v", packet)) // TODO: use packet.String()
	return nil
}

// AcknowledgePacket is called by a module to process the acknowledgement of a
// packet previously sent by the calling module on a channel to a counterparty
// module on the counterparty chain. acknowledgePacket also cleans up the packet
// commitment, which is no longer necessary since the packet has been received
// and acted upon.
func (k Keeper) AcknowledgePacket(
	ctx sdk.Context,
	packet exported.PacketI,
	acknowledgement exported.PacketAcknowledgementI,
	proof commitmentexported.Proof,
	proofHeight uint64,
) (exported.PacketI, error) {
	channel, found := k.GetChannel(ctx, packet.GetSourcePort(), packet.GetSourceChannel())
	if !found {
		return nil, sdkerrors.Wrap(types.ErrChannelNotFound, packet.GetSourceChannel())
	}

	if channel.GetState() != ibctypes.OPEN {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel state is not OPEN (got %s)", channel.GetState().String(),
		)
	}

	// NOTE: RecvPacket is called by the AnteHandler which acts upon the packet.Route(),
	// so the capability authentication can be omitted here

	// packet must have been sent to the channel's counterparty
	if packet.GetDestinationPort() != channel.GetCounterParty().GetPortID() {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet destination port doesn't match the counterparty's port (%s ≠ %s)", packet.GetDestinationPort(), channel.GetCounterParty().GetPortID(),
		)
	}

	if packet.GetDestinationChannel() != channel.GetCounterParty().GetChannelID() {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet destination channel doesn't match the counterparty's channel (%s ≠ %s)", packet.GetDestinationChannel(), channel.GetCounterParty().GetChannelID(),
		)
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.GetConnectionHops()[0])
	if !found {
		return nil, sdkerrors.Wrap(connection.ErrConnectionNotFound, channel.GetConnectionHops()[0])
	}

	if connectionEnd.GetState() != connectionibctypes.OPEN {
		return nil, sdkerrors.Wrapf(
			connection.ErrInvalidConnectionState,
			"connection state is not OPEN (got %s)", connectionEnd.GetState().String(),
		)
	}

	commitment := k.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())

	// verify we sent the packet and haven't cleared it out yet
	if !bytes.Equal(commitment, types.CommitPacket(packet.GetData())) {
		return nil, sdkerrors.Wrap(types.ErrInvalidPacket, "packet hasn't been sent")
	}

	if err := k.connectionKeeper.VerifyPacketAcknowledgement(
		ctx, connectionEnd, proofHeight, proof, packet.GetDestinationPort(), packet.GetDestinationChannel(),
		packet.GetSequence(), acknowledgement.GetBytes(),
	); err != nil {
		return nil, sdkerrors.Wrap(err, "invalid acknowledgement on counterparty chain")
	}

	return packet, nil
}

// CleanupPacket is called by a module to remove a received packet commitment
// from storage. The receiving end must have already processed the packet
// (whether regularly or past timeout).
//
// In the ORDERED channel case, CleanupPacket cleans-up a packet on an ordered
// channel by proving that the packet has been received on the other end.
//
// In the UNORDERED channel case, CleanupPacket cleans-up a packet on an
// unordered channel by proving that the associated acknowledgement has been
//written.
func (k Keeper) CleanupPacket(
	ctx sdk.Context,
	packet exported.PacketI,
	proof commitmentexported.Proof,
	proofHeight,
	nextSequenceRecv uint64,
	acknowledgement []byte,
) (exported.PacketI, error) {
	channel, found := k.GetChannel(ctx, packet.GetSourcePort(), packet.GetSourceChannel())
	if !found {
		return nil, sdkerrors.Wrap(types.ErrChannelNotFound, packet.GetSourceChannel())
	}

	if channel.GetState() != ibctypes.OPEN {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidChannelState,
			"channel state is not OPEN (got %s)", channel.GetState().String(),
		)
	}

	// TODO: blocked by #5542
	// capKey, found := k.GetChannelCapability(ctx, packet.GetSourcePort(), packet.GetSourceChannel())
	// if !found {
	// 	return nil, types.ErrChannelCapabilityNotFound
	// }

	// portCapabilityKey := sdk.NewKVStoreKey(capKey)

	// if !k.portKeeper.Authenticate(portCapabilityKey, packet.GetSourcePort()) {
	// 	return nil, sdkerrors.Wrapf(port.ErrInvalidPort, "invalid source port: %s", packet.GetSourcePort())
	// }

	if packet.GetDestinationPort() != channel.GetCounterParty().GetPortID() {
		return nil, sdkerrors.Wrapf(types.ErrInvalidPacket,
			"packet destination port doesn't match the counterparty's port (%s ≠ %s)", packet.GetDestinationPort(), channel.GetCounterParty().GetPortID(),
		)
	}

	if packet.GetDestinationChannel() != channel.GetCounterParty().GetChannelID() {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidPacket,
			"packet destination channel doesn't match the counterparty's channel (%s ≠ %s)", packet.GetDestinationChannel(), channel.GetCounterParty().GetChannelID(),
		)
	}

	connectionEnd, found := k.connectionKeeper.GetConnection(ctx, channel.GetConnectionHops()[0])
	if !found {
		return nil, sdkerrors.Wrap(connection.ErrConnectionNotFound, channel.GetConnectionHops()[0])
	}

	// check that packet has been received on the other end
	if nextSequenceRecv <= packet.GetSequence() {
		return nil, sdkerrors.Wrap(types.ErrInvalidPacket, "packet already received")
	}

	commitment := k.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())

	// verify we sent the packet and haven't cleared it out yet
	if !bytes.Equal(commitment, types.CommitPacket(packet.GetData())) {
		return nil, sdkerrors.Wrap(types.ErrInvalidPacket, "packet hasn't been sent")
	}

	var err error
	switch channel.GetOrdering() {
	case ibctypes.ORDERED:
		// check that the recv sequence is as claimed
		err = k.connectionKeeper.VerifyNextSequenceRecv(
			ctx, connectionEnd, proofHeight, proof,
			packet.GetDestinationPort(), packet.GetDestinationChannel(), nextSequenceRecv,
		)
	case ibctypes.UNORDERED:
		err = k.connectionKeeper.VerifyPacketAcknowledgement(
			ctx, connectionEnd, proofHeight, proof,
			packet.GetDestinationPort(), packet.GetDestinationChannel(), packet.GetSequence(),
			acknowledgement,
		)
	default:
		panic(sdkerrors.Wrapf(types.ErrInvalidChannelOrdering, channel.GetOrdering().String()))
	}

	if err != nil {
		return nil, sdkerrors.Wrap(err, "packet verification failed")
	}

	k.deletePacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
	return packet, nil
}
