package app

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"google.golang.org/protobuf/proto"

	"shielder/shuttermint/shmsg"
)

func TestQueryInvalidURL(t *testing.T) {
	invalidQueries := []string{
		"http://configs?batchIndex=0",
		"host/configs?batchIndex=0#fragment",
		"user@host/configs?batchIndex=0",
		"/configs?batchIndex=0#fragment",
		"/unknownmethod/",
	}

	app := NewShielderApp()
	for _, path := range invalidQueries {
		req := abcitypes.RequestQuery{
			Path: path,
		}
		res := app.Query(req)
		require.Equal(t, uint32(1), res.Code)
		t.Log(res.Log)
	}
}

func TestQueryInvalidConfig(t *testing.T) {
	invalidQueries := []string{
		"/configs",
		"/configs?batchIndex=-1",
		"/configs?batchIndex=one",
	}

	app := NewShielderApp()
	for _, path := range invalidQueries {
		req := abcitypes.RequestQuery{
			Path: path,
		}
		res := app.Query(req)
		require.Equal(t, uint32(1), res.Code)
		t.Log(res.Log)
	}
}

func TestQueryConfig(t *testing.T) {
	app := NewShielderApp()
	c1 := BatchConfig{
		StartBatchIndex: 100,
		Threshold:       1,
		Keypers: []common.Address{
			common.BigToAddress(big.NewInt(0)),
			common.BigToAddress(big.NewInt(1)),
		},
	}
	c2 := BatchConfig{
		StartBatchIndex: 200,
		Threshold:       1,
		Keypers: []common.Address{
			common.BigToAddress(big.NewInt(2)),
		},
	}
	for _, c := range []BatchConfig{c1, c2} {
		err := app.addConfig(c)
		require.Nil(t, err)
	}

	testCases := []struct {
		path   string
		config BatchConfig
	}{
		{
			path:   "/configs?batchIndex=100",
			config: c1,
		},
		{
			path:   "/configs?batchIndex=199",
			config: c1,
		},
		{
			path:   "/configs?batchIndex=200",
			config: c2,
		},
	}

	for _, testCase := range testCases {
		req := abcitypes.RequestQuery{
			Path: testCase.path,
		}
		res := app.Query(req)
		require.Equal(t, uint32(0), res.Code)

		msg := shmsg.Message{}
		err := proto.Unmarshal(res.Value, &msg)
		require.Nil(t, err)
		batchConfigMsg := msg.GetBatchConfig()
		require.NotNil(t, batchConfigMsg)
		require.Equal(t, batchConfigMsg.StartBatchIndex, testCase.config.StartBatchIndex)
		require.Equal(t, batchConfigMsg.Threshold, testCase.config.Threshold)
		require.Equal(t, len(batchConfigMsg.Keypers), len(testCase.config.Keypers))
		for i := range batchConfigMsg.Keypers {
			address := common.BytesToAddress(batchConfigMsg.Keypers[i])
			require.Equal(t, address, testCase.config.Keypers[i])
		}
	}
}
