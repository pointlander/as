// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"os"
)

func Simulation() {
	var images []*image.Paletted
	img := image.NewGray(image.Rect(0, 0, 8, 8))
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

	animation := &gif.GIF{}
	for _, paletted := range images {
		animation.Image = append(animation.Image, paletted)
		animation.Delay = append(animation.Delay, 0)
	}

	f, _ := os.OpenFile("sim.gif", os.O_WRONLY|os.O_CREATE, 0600)
	defer f.Close()
	gif.EncodeAll(f, animation)
}
