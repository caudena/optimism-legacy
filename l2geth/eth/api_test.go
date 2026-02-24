// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/common/hexutil"
	"github.com/ethereum-optimism/optimism/l2geth/core/rawdb"
	"github.com/ethereum-optimism/optimism/l2geth/core/state"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/crypto"
)

var dumper = spew.ConfigState{Indent: "    "}

func accountRangeTest(t *testing.T, trie *state.Trie, statedb *state.StateDB, start *common.Hash, requestedNum int, expectedNum int) AccountRangeResult {
	result, err := accountRange(*trie, start, requestedNum)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Accounts) != expectedNum {
		t.Fatalf("expected %d results.  Got %d", expectedNum, len(result.Accounts))
	}

	for _, address := range result.Accounts {
		if address == nil {
			t.Fatalf("null address returned")
		}
		if !statedb.Exist(*address) {
			t.Fatalf("account not found in state %s", address.Hex())
		}
	}

	return result
}

type resultHash []*common.Hash

func (h resultHash) Len() int           { return len(h) }
func (h resultHash) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h resultHash) Less(i, j int) bool { return bytes.Compare(h[i].Bytes(), h[j].Bytes()) < 0 }

func TestAccountRange(t *testing.T) {
	var (
		statedb  = state.NewDatabase(rawdb.NewMemoryDatabase())
		state, _ = state.New(common.Hash{}, statedb)
		addrs    = [AccountRangeMaxResults * 2]common.Address{}
		m        = map[common.Address]bool{}
	)

	for i := range addrs {
		hash := common.HexToHash(fmt.Sprintf("%x", i))
		addr := common.BytesToAddress(crypto.Keccak256Hash(hash.Bytes()).Bytes())
		addrs[i] = addr
		state.SetBalance(addrs[i], big.NewInt(1))
		if _, ok := m[addr]; ok {
			t.Fatalf("bad")
		} else {
			m[addr] = true
		}
	}

	state.Commit(true)
	root := state.IntermediateRoot(true)

	trie, err := statedb.OpenTrie(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("test getting number of results less than max")
	accountRangeTest(t, &trie, state, &common.Hash{0x0}, AccountRangeMaxResults/2, AccountRangeMaxResults/2)

	t.Logf("test getting number of results greater than max %d", AccountRangeMaxResults)
	accountRangeTest(t, &trie, state, &common.Hash{0x0}, AccountRangeMaxResults*2, AccountRangeMaxResults)

	t.Logf("test with empty 'start' hash")
	accountRangeTest(t, &trie, state, nil, AccountRangeMaxResults, AccountRangeMaxResults)

	t.Logf("test pagination")

	// test pagination
	firstResult := accountRangeTest(t, &trie, state, &common.Hash{0x0}, AccountRangeMaxResults, AccountRangeMaxResults)

	t.Logf("test pagination 2")
	secondResult := accountRangeTest(t, &trie, state, &firstResult.Next, AccountRangeMaxResults, AccountRangeMaxResults)

	hList := make(resultHash, 0)
	for h1, addr1 := range firstResult.Accounts {
		h := &common.Hash{}
		h.SetBytes(h1.Bytes())
		hList = append(hList, h)
		for h2, addr2 := range secondResult.Accounts {
			// Make sure that the hashes aren't the same
			if bytes.Equal(h1.Bytes(), h2.Bytes()) {
				t.Fatalf("pagination test failed:  results should not overlap")
			}

			// If either address is nil, then it makes no sense to compare
			// them as they might be two different accounts.
			if addr1 == nil || addr2 == nil {
				continue
			}

			// Since the two hashes are different, they should not have
			// the same preimage, but let's check anyway in case there
			// is a bug in the (hash, addr) map generation code.
			if bytes.Equal(addr1.Bytes(), addr2.Bytes()) {
				t.Fatalf("pagination test failed: addresses should not repeat")
			}
		}
	}

	// Test to see if it's possible to recover from the middle of the previous
	// set and get an even split between the first and second sets.
	t.Logf("test random access pagination")
	sort.Sort(hList)
	middleH := hList[AccountRangeMaxResults/2]
	middleResult := accountRangeTest(t, &trie, state, middleH, AccountRangeMaxResults, AccountRangeMaxResults)
	innone, infirst, insecond := 0, 0, 0
	for h := range middleResult.Accounts {
		if _, ok := firstResult.Accounts[h]; ok {
			infirst++
		} else if _, ok := secondResult.Accounts[h]; ok {
			insecond++
		} else {
			innone++
		}
	}
	if innone != 0 {
		t.Fatalf("%d hashes in the 'middle' set were neither in the first not the second set", innone)
	}
	if infirst != AccountRangeMaxResults/2 {
		t.Fatalf("Imbalance in the number of first-test results: %d != %d", infirst, AccountRangeMaxResults/2)
	}
	if insecond != AccountRangeMaxResults/2 {
		t.Fatalf("Imbalance in the number of second-test results: %d != %d", insecond, AccountRangeMaxResults/2)
	}
}

func TestEmptyAccountRange(t *testing.T) {
	var (
		statedb  = state.NewDatabase(rawdb.NewMemoryDatabase())
		state, _ = state.New(common.Hash{}, statedb)
	)

	state.Commit(true)
	root := state.IntermediateRoot(true)

	trie, err := statedb.OpenTrie(root)
	if err != nil {
		t.Fatal(err)
	}

	results, err := accountRange(trie, &common.Hash{0x0}, AccountRangeMaxResults)
	if err != nil {
		t.Fatalf("Empty results should not trigger an error: %v", err)
	}
	if results.Next != common.HexToHash("0") {
		t.Fatalf("Empty results should not return a second page")
	}
	if len(results.Accounts) != 0 {
		t.Fatalf("Empty state should not return addresses: %v", results.Accounts)
	}
}

func TestStorageRangeAt(t *testing.T) {
	// Create a state where account 0x010000... has a few storage entries.
	var (
		state, _ = state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()))
		addr     = common.Address{0x01}
		keys     = []common.Hash{ // hashes of Keys of storage
			common.HexToHash("340dd630ad21bf010b4e676dbfa9ba9a02175262d1fa356232cfde6cb5b47ef2"),
			common.HexToHash("426fcb404ab2d5d8e61a3d918108006bbb0a9be65e92235bb10eefbdb6dcd053"),
			common.HexToHash("48078cfed56339ea54962e72c37c7f588fc4f8e5bc173827ba75cb10a63a96a5"),
			common.HexToHash("5723d2c3a83af9b735e3b7f21531e5623d183a9095a56604ead41f3582fdfb75"),
		}
		storage = storageMap{
			keys[0]: {Key: &common.Hash{0x02}, Value: common.Hash{0x01}},
			keys[1]: {Key: &common.Hash{0x04}, Value: common.Hash{0x02}},
			keys[2]: {Key: &common.Hash{0x01}, Value: common.Hash{0x03}},
			keys[3]: {Key: &common.Hash{0x03}, Value: common.Hash{0x04}},
		}
	)
	for _, entry := range storage {
		state.SetState(addr, *entry.Key, entry.Value)
	}

	// Check a few combinations of limit and start/end.
	tests := []struct {
		start []byte
		limit int
		want  StorageRangeResult
	}{
		{
			start: []byte{}, limit: 0,
			want: StorageRangeResult{storageMap{}, &keys[0]},
		},
		{
			start: []byte{}, limit: 100,
			want: StorageRangeResult{storage, nil},
		},
		{
			start: []byte{}, limit: 2,
			want: StorageRangeResult{storageMap{keys[0]: storage[keys[0]], keys[1]: storage[keys[1]]}, &keys[2]},
		},
		{
			start: []byte{0x00}, limit: 4,
			want: StorageRangeResult{storage, nil},
		},
		{
			start: []byte{0x40}, limit: 2,
			want: StorageRangeResult{storageMap{keys[1]: storage[keys[1]], keys[2]: storage[keys[2]]}, &keys[3]},
		},
	}
	for _, test := range tests {
		result, err := storageRangeAt(state.StorageTrie(addr), test.start, test.limit)
		if err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(result, test.want) {
			t.Fatalf("wrong result for range 0x%x.., limit %d:\ngot %s\nwant %s",
				test.start, test.limit, dumper.Sdump(result), dumper.Sdump(&test.want))
		}
	}
}

func TestNormalizeCallTracerResultFlattensCalls(t *testing.T) {
	txTrace := &txTraceResult{
		Result: map[string]interface{}{
			"type":    "CALL",
			"from":    "0x1111",
			"to":      "0x2222",
			"gas":     "0x10",
			"gasUsed": "0x09",
			"input":   "0xaaaa",
			"output":  "0xbbbb",
			"value":   "0x0",
			"calls": []interface{}{
				map[string]interface{}{
					"type":    "STATICCALL",
					"from":    "0x2222",
					"to":      "0x3333",
					"gas":     "0x08",
					"gasUsed": "0x03",
					"input":   "0xcccc",
					"output":  "0xdddd",
				},
				map[string]interface{}{
					"type":    "CREATE",
					"from":    "0x2222",
					"to":      "0x4444",
					"gas":     "0x07",
					"gasUsed": "0x02",
					"input":   "0xeeee",
					"output":  "0xffff",
					"calls": []interface{}{
						map[string]interface{}{
							"type":    "CALL",
							"from":    "0x4444",
							"to":      "0x5555",
							"gas":     "0x04",
							"gasUsed": "0x01",
							"input":   "0x12",
							"output":  "0x34",
							"value":   "0x1",
						},
					},
				},
			},
		},
	}

	normalized := normalizeCallTracerResult(txTrace)
	if normalized["output"] != "0xbbbb" {
		t.Fatalf("unexpected output: %v", normalized["output"])
	}
	trace, ok := normalized["trace"].([]interface{})
	if !ok {
		t.Fatalf("trace field has wrong type: %T", normalized["trace"])
	}
	if len(trace) != 4 {
		t.Fatalf("unexpected trace length: %d", len(trace))
	}

	rootEntry := trace[0].(map[string]interface{})
	if rootEntry["type"] != "call" {
		t.Fatalf("unexpected root type: %v", rootEntry["type"])
	}
	if !reflect.DeepEqual(rootEntry["traceAddress"], []int{}) {
		t.Fatalf("unexpected root traceAddress: %v", rootEntry["traceAddress"])
	}
	rootAction := rootEntry["action"].(map[string]interface{})
	if rootAction["callType"] != "call" {
		t.Fatalf("unexpected root callType: %v", rootAction["callType"])
	}
	if rootEntry["subtraces"] != 2 {
		t.Fatalf("unexpected root subtraces: %v", rootEntry["subtraces"])
	}

	staticEntry := trace[1].(map[string]interface{})
	if staticEntry["type"] != "call" {
		t.Fatalf("unexpected static type: %v", staticEntry["type"])
	}
	if !reflect.DeepEqual(staticEntry["traceAddress"], []int{0}) {
		t.Fatalf("unexpected static traceAddress: %v", staticEntry["traceAddress"])
	}
	staticAction := staticEntry["action"].(map[string]interface{})
	if staticAction["callType"] != "staticcall" {
		t.Fatalf("unexpected static callType: %v", staticAction["callType"])
	}

	createEntry := trace[2].(map[string]interface{})
	if createEntry["type"] != "create" {
		t.Fatalf("unexpected create type: %v", createEntry["type"])
	}
	if !reflect.DeepEqual(createEntry["traceAddress"], []int{1}) {
		t.Fatalf("unexpected create traceAddress: %v", createEntry["traceAddress"])
	}
	createAction := createEntry["action"].(map[string]interface{})
	if createAction["init"] != "0xeeee" {
		t.Fatalf("unexpected create init: %v", createAction["init"])
	}
	if createEntry["subtraces"] != 1 {
		t.Fatalf("unexpected create subtraces: %v", createEntry["subtraces"])
	}

	nestedEntry := trace[3].(map[string]interface{})
	if !reflect.DeepEqual(nestedEntry["traceAddress"], []int{1, 0}) {
		t.Fatalf("unexpected nested traceAddress: %v", nestedEntry["traceAddress"])
	}
}

func TestNormalizeCallTracerResultPropagatesNodeError(t *testing.T) {
	txTrace := &txTraceResult{
		Result: map[string]interface{}{
			"type":    "CALL",
			"from":    "0x1111",
			"to":      "0x2222",
			"gas":     "0x10",
			"gasUsed": "0x09",
			"input":   "0xaaaa",
			"output":  "0xbbbb",
			"value":   "0x0",
			"error":   "execution reverted",
		},
	}

	normalized := normalizeCallTracerResult(txTrace)
	trace := normalized["trace"].([]interface{})
	if len(trace) != 1 {
		t.Fatalf("unexpected trace length: %d", len(trace))
	}
	entry := trace[0].(map[string]interface{})
	if entry["error"] != "execution reverted" {
		t.Fatalf("unexpected entry error: %v", entry["error"])
	}
}

func TestNormalizeCallTracerResultPropagatesTraceError(t *testing.T) {
	normalized := normalizeCallTracerResult(&txTraceResult{Error: "tracing failed"})
	if normalized["error"] != "tracing failed" {
		t.Fatalf("unexpected trace error: %v", normalized["error"])
	}
	trace := normalized["trace"].([]interface{})
	if len(trace) != 0 {
		t.Fatalf("expected empty trace, got %d entries", len(trace))
	}
}

func TestNormalizeCallTracerResultFromRawJSON(t *testing.T) {
	raw, err := json.Marshal(map[string]interface{}{
		"type":    "CALL",
		"from":    "0x1111",
		"to":      "0x2222",
		"gas":     "0x10",
		"gasUsed": "0x09",
		"input":   "0xaaaa",
		"output":  "0xbbbb",
		"value":   "0x0",
		"calls": []interface{}{
			map[string]interface{}{
				"type":    "DELEGATECALL",
				"from":    "0x2222",
				"to":      "0x3333",
				"gas":     "0x08",
				"gasUsed": "0x03",
				"input":   "0xcccc",
				"output":  "0xdddd",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	normalized := normalizeCallTracerResult(&txTraceResult{Result: json.RawMessage(raw)})
	trace, ok := normalized["trace"].([]interface{})
	if !ok {
		t.Fatalf("trace field has wrong type: %T", normalized["trace"])
	}
	if len(trace) != 2 {
		t.Fatalf("unexpected trace length: %d", len(trace))
	}
	child := trace[1].(map[string]interface{})
	action := child["action"].(map[string]interface{})
	if action["callType"] != "delegatecall" {
		t.Fatalf("unexpected child callType: %v", action["callType"])
	}
}

func TestCompressedPubKeyFromTx(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	to := common.HexToAddress("0x1000000000000000000000000000000000000001")
	unsignedTx := types.NewTransaction(1, to, big.NewInt(1), 21000, big.NewInt(2), []byte{0x1, 0x2, 0x3})
	chainID := big.NewInt(10)
	signedTx, err := types.SignTx(unsignedTx, types.NewEIP155Signer(chainID), key)
	if err != nil {
		t.Fatal(err)
	}

	pubKey, err := compressedPubKeyFromTx(signedTx)
	if err != nil {
		t.Fatal(err)
	}
	expected := crypto.CompressPubkey(&key.PublicKey)
	if !bytes.Equal(pubKey, expected) {
		t.Fatalf("unexpected pubkey: got %x want %x", pubKey, expected)
	}
}

func TestCompressedPubKeyFromUnsignedTxIsZero(t *testing.T) {
	to := common.HexToAddress("0x1000000000000000000000000000000000000001")
	tx := types.NewTransaction(1, to, big.NewInt(1), 21000, big.NewInt(2), []byte{0x1, 0x2, 0x3})

	pubKey, err := compressedPubKeyFromTx(tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pubKey) != 33 {
		t.Fatalf("unexpected pubkey length: %d", len(pubKey))
	}
	if !bytes.Equal(pubKey, make([]byte, 33)) {
		t.Fatalf("expected zero pubkey, got %x", pubKey)
	}
}

func TestRPCLogsWithTimestamp(t *testing.T) {
	logs := []*types.Log{
		{
			Address:     common.HexToAddress("0x1000000000000000000000000000000000000001"),
			Topics:      []common.Hash{common.HexToHash("0x01")},
			Data:        []byte{0xaa},
			BlockNumber: 7,
			TxHash:      common.HexToHash("0x02"),
			TxIndex:     1,
			BlockHash:   common.HexToHash("0x03"),
			Index:       2,
			Removed:     false,
		},
	}
	out := rpcLogsWithTimestamp(logs, 12345)
	if len(out) != 1 {
		t.Fatalf("unexpected logs length: %d", len(out))
	}
	if out[0]["blockTimestamp"] != hexutil.Uint64(12345) {
		t.Fatalf("unexpected blockTimestamp: %v", out[0]["blockTimestamp"])
	}
	data, ok := out[0]["data"].(hexutil.Bytes)
	if !ok {
		t.Fatalf("unexpected data type: %T", out[0]["data"])
	}
	if data.String() != "0xaa" {
		t.Fatalf("unexpected data: %s", data.String())
	}
}
