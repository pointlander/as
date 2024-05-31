// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"math/rand"
	"os"
)

// Simulation mode
func Simulation() {
	const (
		Width     = 16
		Height    = 16
		Particles = 3
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

	sensor := KSensor{}
	var mindX [Particles]MarkovMind
	var mindY [Particles]MarkovMind
	var action [Particles]MarkovMind
	for i := 0; i < Particles; i++ {
		mindX[i] = NewMarkovMind(rng, Width)
		mindY[i] = NewMarkovMind(rng, Height)
		action[i] = NewMarkovMind(rng, 255)
	}
	for i := 0; i < 1024; i++ {
		entropy := sensor.Sense(rng, img)
		for i := 0; i < Particles; i++ {
			actionX := mindX[i].Step(rng, entropy)
			actionY := mindY[i].Step(rng, entropy)
			act := action[i].Step(rng, entropy)
			value := img.GrayAt(actionX, actionY)
			value.Y += byte(act)
			img.SetGray(actionX, actionY, value)
		}
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
