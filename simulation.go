// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"math/cmplx"
	"math/rand"
	"os"

	"github.com/mjibson/go-dsp/dsputils"
	"github.com/mjibson/go-dsp/fft"
	"github.com/pointlander/compress"
)

// Simulation mode
func Simulation() {
	const (
		Width  = 16
		Height = 16
	)
	rng := rand.New(rand.NewSource(1))

	var images []*image.Paletted
	add := func(img image.Image) {
		opts := gif.Options{
			NumColors: 256,
			Drawer:    draw.FloydSteinberg,
		}
		bounds := img.Bounds()
		paletted := image.NewPaletted(bounds, palette.Plan9[:opts.NumColors])
		if opts.Quantizer != nil {
			paletted.Palette = opts.Quantizer.Quantize(make(color.Palette, 0, opts.NumColors), img)
		}
		opts.Drawer.Draw(paletted, bounds, img, image.Point{})
		images = append(images, paletted)
	}

	img := image.NewGray(image.Rect(0, 0, Width, Height))
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			value := color.Gray{}
			if rng.Intn(3) == 0 {
				value.Y = 0
			} else {
				value.Y = 255
			}
			img.SetGray(x, y, value)
		}
	}
	mindX := NewMarkovMind(rng, Width)
	mindY := NewMarkovMind(rng, Height)
	var imgBuffer *dsputils.Matrix
	for i := 0; i < 1024; i++ {
		dx := img.Bounds().Dx()
		dy := img.Bounds().Dy()
		if imgBuffer == nil {
			imgBuffer = dsputils.MakeMatrix(make([]complex128, FFTDepth*dx*dy), []int{FFTDepth, dx, dy})
		}
		for d := FFTDepth - 1; d > 0; d-- {
			for x := 0; x < dx; x++ {
				for y := 0; y < dy; y++ {
					imgBuffer.SetValue(imgBuffer.Value([]int{d - 1, x, y}), []int{d, x, y})
				}
			}
		}
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				g := img.GrayAt(x, y)
				imgBuffer.SetValue(complex(float64(g.Y)/256, 0), []int{0, x, y})
			}
		}
		freq := fft.FFTN(imgBuffer)
		sum := 0.0
		for i := 0; i < FFTDepth; i++ {
			for x := 0; x < dx; x++ {
				for y := 0; y < dy; y++ {
					sum += cmplx.Abs(freq.Value([]int{i, x, y}))
				}
			}
		}
		//entropy := 0.0
		state, index := make([]byte, FFTDepth*dx*dy), 0
		for i := 0; i < FFTDepth; i++ {
			for x := 0; x < dx; x++ {
				for y := 0; y < dy; y++ {
					value := cmplx.Abs(freq.Value([]int{i, x, y})) / sum
					state[index] = byte(255 * value)
					/*if value > 0 {
						entropy += value * math.Log2(value)
					}*/
				}
			}
		}
		output := bytes.Buffer{}
		compress.Mark1Compress1(state, &output)
		//entropy = -entropy
		entropy := 255 * float64(output.Len()) / float64(len(state))
		actionX := mindX.Step(rng, entropy)
		actionY := mindY.Step(rng, entropy)
		value := img.GrayAt(actionX, actionY)
		if value.Y < 128 {
			value.Y = 255
		} else {
			value.Y = 0
		}
		img.SetGray(actionX, actionY, value)
		add(img)
	}

	animation := &gif.GIF{}
	for _, paletted := range images {
		animation.Image = append(animation.Image, paletted)
		animation.Delay = append(animation.Delay, 0)
	}

	f, _ := os.OpenFile("sim.gif", os.O_WRONLY|os.O_CREATE, 0600)
	defer f.Close()
	gif.EncodeAll(f, animation)
}
