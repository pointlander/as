// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"image"
	"image/color"
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

	gray := make([]color.Color, 0, 256)
	for i := 0; i < 256; i++ {
		gray = append(gray, color.GrayModel.Convert(color.Gray{Y: byte(i)}))
	}
	opts := gif.Options{
		NumColors: 256,
		Drawer:    draw.FloydSteinberg,
	}
	var images []*image.Paletted
	add := func(img image.Image) {
		bounds := img.Bounds()
		paletted := image.NewPaletted(bounds, gray)
		opts.Drawer.Draw(paletted, bounds, img, image.Point{})
		images = append(images, paletted)
	}

	img := image.NewGray(image.Rect(0, 0, Width, Height))
	for x := 0; x < Width; x++ {
		for y := 0; y < Height; y++ {
			value := color.Gray{}
			value.Y = byte(rng.Intn(256))
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
					index++
				}
			}
		}
		output := bytes.Buffer{}
		compress.Mark1Compress1(state, &output)
		entropy := 255 * float64(output.Len()) / float64(len(state))
		actionX := mindX.Step(rng, entropy)
		actionY := mindY.Step(rng, entropy)
		value := img.GrayAt(actionX, actionY)
		value.Y = (value.Y + 16) % 255
		img.SetGray(actionX, actionY, value)
		//img.SetGray(rng.Intn(Width), rng.Intn(Height), color.Gray{Y: byte(rng.Intn(256))})
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
