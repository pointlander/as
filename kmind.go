// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"math"
	"math/rand"

	"github.com/pointlander/compress"
)

// KMind is a kolmogorov complexity mind
type KMind struct {
	ActionBuffer []byte
	ActionState  []byte
	StateIndex   int
	ActionIndex  int
	Filter       []float64
}

// NewKMind creates a new kolmogorv mind
func NewKMind(rng *rand.Rand) KMind {
	actionBuffer := make([]byte, Size)
	actionState := make([]byte, Size)
	for i := range actionState {
		actionState[i] = byte(rng.Intn(256))
		actionBuffer[i] = byte(rng.Intn(256))
	}
	filter := make([]float64, ActionCount)
	return KMind{
		ActionBuffer: actionBuffer,
		ActionState:  actionState,
		StateIndex:   0,
		ActionIndex:  1,
		Filter:       filter,
	}
}

// KMind steps the kolmogorov complexity mind
func (k *KMind) Step(rng *rand.Rand, entropy float64) int {
	k.StateIndex = (k.StateIndex + 2) % Size
	k.ActionState[k.StateIndex] = byte(math.Round(entropy))
	k.ActionIndex = (k.ActionIndex + 2) % Size
	entropies := make([]float64, ActionCount)
	for a := 0; a < int(ActionCount); a++ {
		pre := byte(a)
		for i, value := range k.ActionBuffer[:len(k.ActionBuffer)-1] {
			k.ActionBuffer[i], pre = pre, value
		}
		output := bytes.Buffer{}
		compress.Mark1Compress1(k.ActionBuffer, &output)
		entropy := 256 * float64(output.Len()) / Size
		k.ActionState[k.ActionIndex] = byte(math.Round(entropy))
		output = bytes.Buffer{}
		compress.Mark1Compress1(k.ActionState, &output)
		entropies[a] = float64(output.Len()) / Size
	}
	for i, value := range entropies {
		k.Filter[i] = (k.Filter[i] + value) / 2
	}
	normalized := softmax(k.Filter, .4)
	sum, action, selected := 0.0, 0, rng.Float64()
	for i, value := range normalized {
		sum += value
		if sum > selected {
			action = i
			break
		}
	}
	k.ActionState[k.ActionIndex] = byte(math.Round(256 * entropies[action]))
	k.ActionBuffer[0] = byte(action)
	return action
}
