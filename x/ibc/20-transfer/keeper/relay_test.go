package keeper_test

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	channeltypes "github.com/cosmos/cosmos-sdk/x/ibc/04-channel/types"
	"github.com/cosmos/cosmos-sdk/x/ibc/20-transfer/types"
	"github.com/cosmos/cosmos-sdk/x/supply"
)

func (suite *KeeperTestSuite) TestSendTransfer() {
	testCoins2 := sdk.NewCoins(sdk.NewCoin("testportid/secondchannel/atom", sdk.NewInt(100)))
	testCases := []struct {
		msg           string
		amount        sdk.Coins
		malleate      func()
		isSourceChain bool
		expPass       bool
	}{
		{"successful transfer from source chain", testCoins,
			func() {
				suite.chainA.App.BankKeeper.AddCoins(suite.chainA.GetContext(), testAddr1, testCoins)
				suite.chainA.CreateClient(suite.chainB)
				suite.chainA.createConnection(testConnection, testConnection, testClientIDB, testClientIDA, connectionibctypes.OPEN)
				suite.chainA.createChannel(testPort1, testChannel1, testPort2, testChannel2, channelibctypes.OPEN, channelibctypes.ORDERED, testConnection)
				suite.chainA.App.IBCKeeper.ChannelKeeper.SetNextSequenceSend(suite.chainA.GetContext(), testPort1, testChannel1, 1)
			}, true, true},
		{"successful transfer from source chain with denom prefix", testCoins2,
			func() {
				_, err := suite.chainA.App.BankKeeper.AddCoins(suite.chainA.GetContext(), testAddr1, testCoins)
				suite.Require().NoError(err)
				suite.chainA.CreateClient(suite.chainB)
				suite.chainA.createConnection(testConnection, testConnection, testClientIDB, testClientIDA, connectionibctypes.OPEN)
				suite.chainA.createChannel(testPort1, testChannel1, testPort2, testChannel2, channelibctypes.OPEN, channelibctypes.ORDERED, testConnection)
				suite.chainA.App.IBCKeeper.ChannelKeeper.SetNextSequenceSend(suite.chainA.GetContext(), testPort1, testChannel1, 1)
			}, true, true},
		{"successful transfer from external chain", testCoins,
			func() {
				suite.chainA.App.SupplyKeeper.SetSupply(suite.chainA.GetContext(), supply.NewSupply(prefixCoins))
				_, err := suite.chainA.App.BankKeeper.AddCoins(suite.chainA.GetContext(), testAddr1, prefixCoins)
				suite.Require().NoError(err)
				suite.chainA.CreateClient(suite.chainB)
				suite.chainA.createConnection(testConnection, testConnection, testClientIDB, testClientIDA, connectionibctypes.OPEN)
				suite.chainA.createChannel(testPort1, testChannel1, testPort2, testChannel2, channelibctypes.OPEN, channelibctypes.ORDERED, testConnection)
				suite.chainA.App.IBCKeeper.ChannelKeeper.SetNextSequenceSend(suite.chainA.GetContext(), testPort1, testChannel1, 1)
			}, false, true},
		{"source channel not found", testCoins,
			func() {}, true, false},
		{"next seq send not found", testCoins,
			func() {
				suite.chainA.CreateClient(suite.chainB)
				suite.chainA.createConnection(testConnection, testConnection, testClientIDB, testClientIDA, connectionibctypes.OPEN)
				suite.chainA.createChannel(testPort1, testChannel1, testPort2, testChannel2, channelibctypes.OPEN, channelibctypes.ORDERED, testConnection)
			}, true, false},
		// createOutgoingPacket tests
		// - source chain
		{"send coins failed", testCoins,
			func() {
				suite.chainA.CreateClient(suite.chainB)
				suite.chainA.createConnection(testConnection, testConnection, testClientIDB, testClientIDA, connectionibctypes.OPEN)
				suite.chainA.createChannel(testPort1, testChannel1, testPort2, testChannel2, channelibctypes.OPEN, channelibctypes.ORDERED, testConnection)
				suite.chainA.App.IBCKeeper.ChannelKeeper.SetNextSequenceSend(suite.chainA.GetContext(), testPort1, testChannel1, 1)
			}, true, false},
		// - receiving chain
		{"send from module account failed", testCoins,
			func() {
				suite.chainA.CreateClient(suite.chainB)
				suite.chainA.createConnection(testConnection, testConnection, testClientIDB, testClientIDA, connectionibctypes.OPEN)
				suite.chainA.createChannel(testPort1, testChannel1, testPort2, testChannel2, channelibctypes.OPEN, channelibctypes.ORDERED, testConnection)
				suite.chainA.App.IBCKeeper.ChannelKeeper.SetNextSequenceSend(suite.chainA.GetContext(), testPort1, testChannel1, 1)
			}, false, false},
	}

	for i, tc := range testCases {
		tc := tc
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset

			tc.malleate()

			err := suite.chainA.App.TransferKeeper.SendTransfer(
				suite.chainA.GetContext(), testPort1, testChannel1, 100, tc.amount, testAddr1, testAddr2, tc.isSourceChain,
			)

			if tc.expPass {
				suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.msg)
			} else {
				suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.msg)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestReceiveTransfer() {
	data := types.NewFungibleTokenPacketData(prefixCoins2, testAddr1, testAddr2, true, 100)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{"success receive from source chain",
			func() {}, true},
		// onRecvPacket
		// - source chain
		{"no dest prefix on coin denom",
			func() {
				data.Amount = testCoins
			}, false},
		{"mint failed",
			func() {
				data.Source = true
				data.Amount = prefixCoins2
				data.Amount[0].Amount = sdk.ZeroInt()
			}, false},
		// - receiving chain
		{"incorrect dest prefix on coin denom",
			func() {
				data.Source = false
				data.Amount = prefixCoins2
			}, false},
		{"success receive from external chain",
			func() {
				data.Source = false
				data.Amount = prefixCoins
				escrow := types.GetEscrowAddress(testPort2, testChannel2)
				_, err := suite.chainA.App.BankKeeper.AddCoins(suite.chainA.GetContext(), escrow, testCoins)
				suite.Require().NoError(err)
			}, true},
	}

	packet := channeltypes.NewPacket(data, 1, testPort1, testChannel1, testPort2, testChannel2)

	for i, tc := range testCases {
		tc := tc
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset
			tc.malleate()

			err := suite.chainA.App.TransferKeeper.ReceiveTransfer(suite.chainA.GetContext(), packet, data)

			if tc.expPass {
				suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.msg)
			} else {
				suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.msg)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestTimeoutTransfer() {
	data := types.NewFungibleTokenPacketData(prefixCoins, testAddr1, testAddr2, true, 100)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{"successful timeout from source chain",
			func() {
				escrow := types.GetEscrowAddress(testPort2, testChannel2)
				_, err := suite.chainA.App.BankKeeper.AddCoins(suite.chainA.GetContext(), escrow, sdk.NewCoins(sdk.NewCoin("atom", sdk.NewInt(100))))
				suite.Require().NoError(err)
			}, true},
		{"successful timeout from external chain",
			func() {
				data.Source = false
			}, true},
		{"no source prefix on coin denom",
			func() {
				data.Source = true
				data.Amount = prefixCoins2
			}, false},
		{"unescrow failed",
			func() {
				data.Source = true
			}, false},
		{"mint failed",
			func() {
				data.Source = false
				data.Amount[0].Denom = prefixCoins[0].Denom
				data.Amount[0].Amount = sdk.ZeroInt()
			}, false},
	}

	packet := channeltypes.NewPacket(data, 1, testPort1, testChannel1, testPort2, testChannel2)

	for i, tc := range testCases {
		tc := tc
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset
			tc.malleate()

			err := suite.chainA.App.TransferKeeper.TimeoutTransfer(suite.chainA.GetContext(), packet, data)

			if tc.expPass {
				suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.msg)
			} else {
				suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.msg)
			}
		})
	}
}
