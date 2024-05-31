// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"image"
	"math"
	"math/cmplx"

	"github.com/mjibson/go-dsp/dsputils"
	"github.com/mjibson/go-dsp/fft"
)

// ESensor is an entropy sensor
type ESensor struct {
	ImgBuffer *dsputils.Matrix
}

// Sense senses an image
func (e *ESensor) Sense(img *image.Gray) float64 {
	dx := img.Bounds().Dx()
	dy := img.Bounds().Dy()
	if e.ImgBuffer == nil {
		e.ImgBuffer = dsputils.MakeMatrix(make([]complex128, FFTDepth*dx*dy), []int{FFTDepth, dx, dy})
	}
	for d := FFTDepth - 1; d > 0; d-- {
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				e.ImgBuffer.SetValue(e.ImgBuffer.Value([]int{d - 1, x, y}), []int{d, x, y})
			}
		}
	}
	for x := 0; x < dx; x++ {
		for y := 0; y < dy; y++ {
			g := img.GrayAt(x, y)
			e.ImgBuffer.SetValue(complex(float64(g.Y)/256, 0), []int{0, x, y})
		}
	}
	freq := fft.FFTN(e.ImgBuffer)
	sum := 0.0
	for i := 0; i < FFTDepth; i++ {
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				sum += cmplx.Abs(freq.Value([]int{i, x, y}))
			}
		}
	}
	entropy := 0.0
	for i := 0; i < FFTDepth; i++ {
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				value := cmplx.Abs(freq.Value([]int{i, x, y})) / sum
				entropy += value * math.Log2(value)
			}
		}
	}
	return -entropy
}
