// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"image"
	"math"
	"math/cmplx"
	"math/rand"

	"github.com/mjibson/go-dsp/dsputils"
	"github.com/mjibson/go-dsp/fft"
	"github.com/pointlander/compress"
)

// KSensor is a kolmogorov sensor
type KSensor struct {
	ImgBuffer *dsputils.Matrix
}

// Sense senses an image
func (k *KSensor) Sense(rng *rand.Rand, img *image.Gray) float64 {
	dx := img.Bounds().Dx()
	dy := img.Bounds().Dy()
	if k.ImgBuffer == nil {
		k.ImgBuffer = dsputils.MakeMatrix(make([]complex128, FFTDepth*dx*dy), []int{FFTDepth, dx, dy})
	}
	for d := FFTDepth - 1; d > 0; d-- {
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				k.ImgBuffer.SetValue(k.ImgBuffer.Value([]int{d - 1, x, y}), []int{d, x, y})
			}
		}
	}
	for x := 0; x < dx; x++ {
		for y := 0; y < dy; y++ {
			g := float64(img.GrayAt(x, y).Y)
			if rng != nil {
				g += 3 * rng.NormFloat64()
				if g < 0 {
					g = 0
				} else if g > 255 {
					g = 255
				}
			}
			k.ImgBuffer.SetValue(complex(g/255, 0), []int{0, x, y})
		}
	}
	freq := fft.FFTN(k.ImgBuffer)
	sum := 0.0
	sumPhase := 0.0
	for i := 0; i < FFTDepth; i++ {
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				value := freq.Value([]int{i, x, y})
				sum += cmplx.Abs(value)
				sumPhase += cmplx.Phase(value) + math.Pi
			}
		}
	}
	state, index := make([]byte, 2*FFTDepth*dx*dy), 0
	for i := 0; i < FFTDepth; i++ {
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				value := freq.Value([]int{i, x, y})
				state[index] = byte(255 * cmplx.Abs(value) / sum)
				index++
				state[index] = byte(255 * (cmplx.Phase(value) + math.Pi) / sumPhase)
				index++
			}
		}
	}
	output := bytes.Buffer{}
	compress.Mark1Compress1(state, &output)
	entropy := 255 * float64(output.Len()) / float64(len(state))
	return entropy
}
