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

// Package tracers is a collection of JavaScript transaction tracers.
package tracers

import (
	"strings"
	"unicode"

	"github.com/ethereum-optimism/optimism/l2geth/eth/tracers/internal/tracers"
)

// all contains all the built in JavaScript tracers by name.
var all = make(map[string]string)

const oldSelfdestructBlock = `		// If a contract is being self destructed, gather that as a subcall too
		if (syscall && op == 'SELFDESTRUCT') {
			var left = this.callstack.length;
			if (this.callstack[left-1].calls === undefined) {
				this.callstack[left-1].calls = [];
			}
			this.callstack[left-1].calls.push({type: op});
			return
		}
`

const newSelfdestructBlock = `		// If a contract is being self destructed, gather that as a subcall too
		if (syscall && op == 'SELFDESTRUCT') {
			var left = this.callstack.length;
			if (this.callstack[left-1].calls === undefined) {
				this.callstack[left-1].calls = [];
			}
			var address = log.contract.getAddress();
			var refundAddress = toAddress(log.stack.peek(0).toString(16));
			this.callstack[left-1].calls.push({
				type:  op,
				from:  toHex(address),
				to:    toHex(refundAddress),
				value: '0x' + db.getBalance(address).toString(16)
			});
			return
		}
`

// camel converts a snake cased input string into a camel cased output.
func camel(str string) string {
	pieces := strings.Split(str, "_")
	for i := 1; i < len(pieces); i++ {
		pieces[i] = string(unicode.ToUpper(rune(pieces[i][0]))) + pieces[i][1:]
	}
	return strings.Join(pieces, "")
}

// init retrieves the JavaScript transaction tracers included in go-ethereum.
func init() {
	for _, file := range tracers.AssetNames() {
		name := camel(strings.TrimSuffix(file, ".js"))
		all[name] = string(tracers.MustAsset(file))
	}
	// Backport SELFDESTRUCT details to the legacy call tracer so consumers can map
	// parity-style suicide actions without synthetic zero addresses/balances.
	if code, ok := all["callTracer"]; ok {
		all["callTracer"] = strings.Replace(code, oldSelfdestructBlock, newSelfdestructBlock, 1)
	}
}

// tracer retrieves a specific JavaScript tracer by name.
func tracer(name string) (string, bool) {
	if tracer, ok := all[name]; ok {
		return tracer, true
	}
	return "", false
}
