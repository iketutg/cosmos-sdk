package types_test

import (
	"testing"
	"time"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/msg_authorization/types"
	"github.com/stretchr/testify/require"
)

var (
	coinsPos = sdk.NewCoins(sdk.NewInt64Coin("steak", 100))
	granter  = sdk.AccAddress("_______granter______")
	grantee  = sdk.AccAddress("_______grantee______")
)

func TestMsgExecAuthorized(t *testing.T) {
	tests := []struct {
		title      string
		grantee    sdk.AccAddress
		msgs       []sdk.Msg
		expectPass bool
	}{
		{"nil grantee address", nil, []sdk.Msg{}, false},
		{"valid test", grantee, []sdk.Msg{}, true},
		{"valid test: msg type", grantee, []sdk.Msg{&banktypes.MsgSend{
			Amount:      sdk.NewCoins(sdk.NewInt64Coin("steak", 2)),
			FromAddress: granter.String(),
			ToAddress:   grantee.String(),
		}}, true},
	}
	for i, tc := range tests {
		msg := types.NewMsgExecAuthorized(tc.grantee, tc.msgs)
		if tc.expectPass {
			require.NoError(t, msg.ValidateBasic(), "test: %v", i)
		} else {
			require.Error(t, msg.ValidateBasic(), "test: %v", i)
		}
	}
}
func TestMsgRevokeAuthorization(t *testing.T) {
	tests := []struct {
		title            string
		granter, grantee sdk.AccAddress
		msgType          string
		expectPass       bool
	}{
		{"nil Granter address", nil, grantee, "hello", false},
		{"nil Grantee address", granter, nil, "hello", false},
		{"nil Granter and Grantee address", nil, nil, "hello", false},
		{"valid test case", granter, grantee, "hello", true},
	}
	for i, tc := range tests {
		msg := types.NewMsgRevokeAuthorization(tc.granter, tc.grantee, tc.msgType)
		if tc.expectPass {
			require.NoError(t, msg.ValidateBasic(), "test: %v", i)
		} else {
			require.Error(t, msg.ValidateBasic(), "test: %v", i)
		}
	}
}

func TestMsgGrantAuthorization(t *testing.T) {
	tests := []struct {
		title            string
		granter, grantee sdk.AccAddress
		authorization    types.Authorization
		expiration       time.Time
		expectErr        bool
		expectPass       bool
	}{
		{"nil granter address", nil, grantee, &types.SendAuthorization{SpendLimit: coinsPos}, time.Now(), false, false},
		{"nil grantee address", granter, nil, &types.SendAuthorization{SpendLimit: coinsPos}, time.Now(), false, false},
		{"nil granter and grantee address", nil, nil, &types.SendAuthorization{SpendLimit: coinsPos}, time.Now(), false, false},
		{"nil authorization", granter, grantee, nil, time.Now(), true, false},
		{"valid test case", granter, grantee, &types.SendAuthorization{SpendLimit: coinsPos}, time.Now(), false, true},
		{"past time", granter, grantee, &types.SendAuthorization{SpendLimit: coinsPos}, time.Now().AddDate(0, 0, -1), false, false},
	}
	for i, tc := range tests {
		msg, err := types.NewMsgGrantAuthorization(
			tc.granter, tc.grantee, tc.authorization, tc.expiration,
		)
		if !tc.expectErr {
			require.NoError(t, err)
		} else {
			continue
		}
		if tc.expectPass {
			require.NoError(t, msg.ValidateBasic(), "test: %v", i)
		} else {
			require.Error(t, msg.ValidateBasic(), "test: %v", i)
		}
	}
}
