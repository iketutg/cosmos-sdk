package types

import (
	"strings"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	host "github.com/cosmos/cosmos-sdk/x/ibc/24-host"
	ibctypes "github.com/cosmos/cosmos-sdk/x/ibc/types"
)

// NewChannel creates a new Channel instance
func NewChannel(
	state ibctypes.State, ordering ibctypes.Order, counterparty Counterparty,
	hops []string, version string,
) Channel {
	return Channel{
		State:          state,
		Ordering:       ordering,
		Counterparty:   counterparty,
		ConnectionHops: hops,
		Version:        version,
	}
}

// ValidateBasic performs a basic validation of the channel fields
func (ch Channel) ValidateBasic() error {
	if ch.State.String() == "" {
		return sdkerrors.Wrap(ErrInvalidChannel, ErrInvalidChannelState.Error())
	}
	if ch.Ordering.String() == "" {
		return sdkerrors.Wrap(ErrInvalidChannel, ErrInvalidChannelOrdering.Error())
	}
	if len(ch.ConnectionHops) != 1 {
		return sdkerrors.Wrap(
			ErrInvalidChannel,
			sdkerrors.Wrap(ErrTooManyConnectionHops, "IBC v1.0 only supports one connection hop").Error(),
		)
	}
	if err := host.DefaultConnectionIdentifierValidator(ch.ConnectionHops[0]); err != nil {
		return sdkerrors.Wrap(
			ErrInvalidChannel,
			sdkerrors.Wrap(err, "invalid connection hop ID").Error(),
		)
	}
	if strings.TrimSpace(ch.Version) == "" {
		return sdkerrors.Wrap(
			ErrInvalidChannel,
			sdkerrors.Wrap(ibctypes.ErrInvalidVersion, "channel version can't be blank").Error(),
		)
	}
	return ch.Counterparty.ValidateBasic()
}

// NewCounterparty returns a new Counterparty instance
func NewCounterparty(portID, channelID string) Counterparty {
	return Counterparty{
		PortID:    portID,
		ChannelID: channelID,
	}
}

// ValidateBasic performs a basic validation check of the identifiers
func (c Counterparty) ValidateBasic() error {
	if err := host.DefaultPortIdentifierValidator(c.PortID); err != nil {
		return sdkerrors.Wrap(
			ErrInvalidCounterparty,
			sdkerrors.Wrap(err, "invalid counterparty connection ID").Error(),
		)
	}
	if err := host.DefaultChannelIdentifierValidator(c.ChannelID); err != nil {
		return sdkerrors.Wrap(
			ErrInvalidCounterparty,
			sdkerrors.Wrap(err, "invalid counterparty client ID").Error(),
		)
	}
	return nil
}
