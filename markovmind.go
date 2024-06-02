// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"math"
	"math/rand"
)

// Context is a markov context
type Context [3]byte

// MarkovMind is a markov model mind
type MarkovMind struct {
	Actions int
	Acts    []float64
	State   Context
	Markov  map[Context][]float64
}

// NewMarkovMind creates a new markov model mind
func NewMarkovMind(rng *rand.Rand, actions int) MarkovMind {
	return MarkovMind{
		Actions: actions,
		Markov:  make(map[Context][]float64),
	}
}

// Step the markov mind
func (m *MarkovMind) Step(rng *rand.Rand, entropy float64) int {
	s := byte(math.Round(entropy))
	acts := m.Acts
	actions, ok := m.Markov[m.State]
	if !ok {
		actions = make([]float64, m.Actions)
		for key := range actions {
			actions[key] = rng.Float64()
		}
	}
	normalized := softmax(actions, 1)
	sum, selected := 0.0, rng.Float64()
	act := 0
	for i, value := range normalized {
		sum += value
		if sum > selected {
			act = i
			break
		}
	}

	if len(acts) > 0 {
		for a := range actions {
			actions[a] += 1 - acts[a]
		}
		sum = 0.0
		for _, value := range actions {
			sum += value
		}
		for key, value := range actions {
			actions[key] = value / sum
		}
	}
	m.Acts = actions
	m.Markov[m.State] = actions
	m.State[0], m.State[1], m.State[2] = m.State[1], m.State[2], s
	return act
}
