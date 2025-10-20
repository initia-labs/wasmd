package keeper

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/CosmWasm/wasmd/x/wasm/types"
)

func TestSelectAuthorizationPolicy(t *testing.T) {
	myGovAuthority := RandomAccountAddress(t)
	m := msgServer{keeper: &Keeper{
		propagateGovAuthorization: map[types.AuthorizationPolicyAction]struct{}{
			types.AuthZActionMigrateContract: {},
			types.AuthZActionInstantiate:     {},
		},
		authority: myGovAuthority.String(),
	}}

	ms := store.NewCommitMultiStore(dbm.NewMemDB(), log.NewTestLogger(t), storemetrics.NewNoOpMetrics())
	ctx := sdk.NewContext(ms, tmproto.Header{}, false, log.NewNopLogger())

	specs := map[string]struct {
		ctx   sdk.Context
		actor sdk.AccAddress
		exp   types.AuthorizationPolicy
	}{
		"always gov policy for gov authority sender": {
			ctx:   types.WithSubMsgAuthzPolicy(ctx, NewPartialGovAuthorizationPolicy(nil, types.AuthZActionMigrateContract)),
			actor: myGovAuthority,
			exp:   NewGovAuthorizationPolicy(types.AuthZActionMigrateContract, types.AuthZActionInstantiate),
		},
		"pick from context when set": {
			ctx:   types.WithSubMsgAuthzPolicy(ctx, NewPartialGovAuthorizationPolicy(nil, types.AuthZActionMigrateContract)),
			actor: RandomAccountAddress(t),
			exp:   NewPartialGovAuthorizationPolicy(nil, types.AuthZActionMigrateContract),
		},
		"fallback to default policy": {
			ctx:   ctx,
			actor: RandomAccountAddress(t),
			exp:   DefaultAuthorizationPolicy{},
		},
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			got := m.selectAuthorizationPolicy(spec.ctx, spec.actor.String())
			assert.Equal(t, spec.exp, got)
		})
	}
}

func TestMsgUpdateMaxWasmSize(t *testing.T) {
	ctx, keepers := CreateTestInput(t, false, AvailableCapabilities)
	govAuthority := keepers.WasmKeeper.GetAuthority()
	nonAuthority := RandomAccountAddress(t).String()

	specs := map[string]struct {
		req    *types.MsgUpdateMaxWasmSize
		expErr bool
		errMsg string
	}{
		"valid update by authority": {
			req: &types.MsgUpdateMaxWasmSize{
				Authority:   govAuthority,
				MaxWasmSize: 2 * 1024 * 1024,
			},
			expErr: false,
		},
		"invalid authority": {
			req: &types.MsgUpdateMaxWasmSize{
				Authority:   nonAuthority,
				MaxWasmSize: 2 * 1024 * 1024,
			},
			expErr: true,
			errMsg: "invalid authority",
		},
		"zero max_wasm_size": {
			req: &types.MsgUpdateMaxWasmSize{
				Authority:   govAuthority,
				MaxWasmSize: 0,
			},
			expErr: true,
			errMsg: "max wasm size cannot be zero",
		},
		"max_wasm_size exceeds limit": {
			req: &types.MsgUpdateMaxWasmSize{
				Authority:   govAuthority,
				MaxWasmSize: uint64(types.MaxWasmSizeLimit) + 1,
			},
			expErr: true,
			errMsg: "max wasm size cannot exceed",
		},
		"update to minimum valid size": {
			req: &types.MsgUpdateMaxWasmSize{
				Authority:   govAuthority,
				MaxWasmSize: 1,
			},
			expErr: false,
		},
		"update to maximum valid size": {
			req: &types.MsgUpdateMaxWasmSize{
				Authority:   govAuthority,
				MaxWasmSize: uint64(types.MaxWasmSizeLimit),
			},
			expErr: false,
		},
	}

	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			msgServer := NewMsgServerImpl(keepers.WasmKeeper)
			_, err := msgServer.UpdateMaxWasmSize(ctx, spec.req)

			if spec.expErr {
				require.Error(t, err)
				if spec.errMsg != "" {
					require.Contains(t, err.Error(), spec.errMsg)
				}
			} else {
				require.NoError(t, err)
				params := keepers.WasmKeeper.GetParams(ctx)
				assert.Equal(t, spec.req.MaxWasmSize, params.MaxWasmSize)
			}
		})
	}
}

func TestMsgUpdateMaxWasmSizeEmitsEvent(t *testing.T) {
	ctx, keepers := CreateTestInput(t, false, AvailableCapabilities)
	govAuthority := keepers.WasmKeeper.GetAuthority()

	em := sdk.NewEventManager()
	ctx = ctx.WithEventManager(em)

	msgServer := NewMsgServerImpl(keepers.WasmKeeper)
	newMaxSize := uint64(1024 * 1024)
	_, err := msgServer.UpdateMaxWasmSize(ctx, &types.MsgUpdateMaxWasmSize{
		Authority:   govAuthority,
		MaxWasmSize: newMaxSize,
	})
	require.NoError(t, err)

	// verify event was emitted
	events := em.Events()
	require.Len(t, events, 1)
	assert.Equal(t, types.EventTypeUpdateMaxWasmSize, events[0].Type)

	// verify event attributes
	attrs := events[0].Attributes
	require.Len(t, attrs, 1)
	assert.Equal(t, types.AttributeKeyNewMaxWasmSize, attrs[0].Key)
	assert.Equal(t, "1048576", attrs[0].Value)
}
