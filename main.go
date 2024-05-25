// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"math"
	"math/cmplx"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mjibson/go-dsp/dsputils"
	"github.com/mjibson/go-dsp/fft"
	"github.com/pointlander/compress"
	"github.com/veandco/go-sdl2/sdl"
	"go.bug.st/serial"
)

var joysticks = make(map[int]*sdl.Joystick)

const (
	// S is the scaling factor for the softmax
	S = 1.0 - 1e-300
	// Size is the size of the buffers
	Size = 1024
	// FFTDepth is the depth of the fft
	FFTDepth = 8
)

type (
	// JoystickState is the state of a joystick
	JoystickState uint
	// Mode is the operating mode of the robot
	Mode uint
	// Camera is a camera
	TypeCamera uint
	// Action is an action to take
	TypeAction uint
)

const (
	// JoystickStateNone is the default state of a joystick
	JoystickStateNone JoystickState = iota
	// JoystickStateUp is the state of a joystick when it is pushed up
	JoystickStateUp
	// JoystickStateDown is the state of a joystick when it is pushed down
	JoystickStateDown
)

const (
	// ModeManual
	ModeManual Mode = iota
	// ModeAuto
	ModeAuto
)

const (
	// ActionLeft
	ActionLeft TypeAction = iota
	// ActionRight
	ActionRight
	// ActionForward
	ActionForward
	// ActionBackward
	ActionBackward
	// ActionNone
	ActionNone
	// ActionCount
	ActionCount
)

// String returns a string representation of the JoystickState
func (j JoystickState) String() string {
	switch j {
	case JoystickStateUp:
		return "up"
	case JoystickStateDown:
		return "down"
	default:
		return "none"
	}
}

// Frame is a video frame
type Frame struct {
	Frame *image.YCbCr
	Thumb image.Image
	Gray  *image.Gray
}

func softmax(values []float64, t float64) []float64 {
	output := make([]float64, len(values))
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	s := max * S
	sum := 0.0
	for j, value := range values {
		value /= t
		output[j] = math.Exp(value - s)
		sum += output[j]
	}
	for j, value := range values {
		value /= t
		output[j] = value / sum
	}
	return output
}

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

// Context is a markov context
type Context [2]byte

// MarkovMind is a markov model mind
type MarkovMind struct {
	Actions int
	Action  byte
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
	action := m.Action
	actions, ok := m.Markov[m.State]
	if !ok {
		actions = make([]float64, m.Actions)
		for key := range actions {
			actions[key] = rng.Float64()
		}
	}
	normalized := softmax(actions, 1)
	sum, selected := 0.0, rng.Float64()
	for i, value := range normalized {
		sum += value
		if sum > selected {
			m.Action = byte(i)
			break
		}
	}

	actions[action] += .1
	sum = 0.0
	for _, value := range actions {
		sum += value
	}
	for key, value := range actions {
		actions[key] = value / sum
	}
	m.Markov[m.State] = actions
	m.State[0], m.State[1] = m.State[1], s
	return int(m.Action)
}

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

// KSensor is a kolmogorov sensor
type KSensor struct {
	ImgBuffer *dsputils.Matrix
}

// Sense senses an image
func (k *KSensor) Sense(img *image.Gray) float64 {
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
			g := img.GrayAt(x, y)
			k.ImgBuffer.SetValue(complex(float64(g.Y)/256, 0), []int{0, x, y})
		}
	}
	freq := fft.FFTN(k.ImgBuffer)
	sum := 0.0
	for i := 0; i < FFTDepth; i++ {
		for x := 0; x < dx; x++ {
			for y := 0; y < dy; y++ {
				sum += cmplx.Abs(freq.Value([]int{i, x, y}))
			}
		}
	}
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
	return entropy
}

var (
	// FlagSim is simulation mode
	FlagSim = flag.Bool("sim", false, "simulation mode")
)

func main() {
	flag.Parse()

	if *FlagSim {
		Simulation()
		return
	}

	options := &serial.Mode{
		BaudRate: 115200,
	}
	port, err := serial.Open("/dev/ttyAMA0", options)
	if err != nil {
		panic(err)
	}

	var running bool

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		err := port.Close()
		if err != nil {
			panic(err)
		}
		running = false
		os.Exit(1)
	}()

	a := ActionNone
	camera := NewV4LCamera()
	go camera.Start("/dev/video0")
	go func() {
		rng := rand.New(rand.NewSource(1))
		//mind := NewKMind(rng)
		mind := NewMarkovMind(rng, int(ActionCount))
		sensor := ESensor{}
		for img := range camera.Images {
			entropy := sensor.Sense(img.Gray)
			entropy *= 4
			action := mind.Step(rng, entropy)
			a = TypeAction(action)
		}
	}()

	var event sdl.Event
	sdl.Init(sdl.INIT_JOYSTICK)
	defer sdl.Quit()
	sdl.JoystickEventState(sdl.ENABLE)
	running = true
	var axis [5]int16
	joystickLeft := JoystickStateNone
	joystickRight := JoystickStateNone
	speed := 0.2
	var mode Mode

	go func() {
		message := map[string]interface{}{
			"T":      900,
			"main":   2,
			"module": 0,
		}
		data, err := json.Marshal(message)
		if err != nil {
			panic(err)
		}
		data = append(data, '\n')
		_, err = port.Write(data)
		if err != nil {
			panic(err)
		}
		leftSpeed, rightSpeed := 0.0, 0.0
		for running {
			time.Sleep(300 * time.Millisecond)

			if mode == ModeAuto {
				switch a {
				case ActionForward:
					joystickLeft = JoystickStateUp
					joystickRight = JoystickStateUp
				case ActionBackward:
					joystickLeft = JoystickStateDown
					joystickRight = JoystickStateDown
				case ActionLeft:
					joystickLeft = JoystickStateDown
					joystickRight = JoystickStateUp
				case ActionRight:
					joystickLeft = JoystickStateUp
					joystickRight = JoystickStateDown
				case ActionNone:
					joystickLeft = JoystickStateNone
					joystickRight = JoystickStateNone
				}
			}

			switch joystickLeft {
			case JoystickStateUp:
				leftSpeed = speed
			case JoystickStateDown:
				leftSpeed = -speed
			case JoystickStateNone:
				leftSpeed = 0.0
			}
			switch joystickRight {
			case JoystickStateUp:
				rightSpeed = speed
			case JoystickStateDown:
				rightSpeed = -speed
			case JoystickStateNone:
				rightSpeed = 0.0
			}

			message := map[string]interface{}{
				"T": 1,
				"L": leftSpeed,
				"R": rightSpeed,
			}
			data, err := json.Marshal(message)
			if err != nil {
				panic(err)
			}
			data = append(data, '\n')
			_, err = port.Write(data)
			if err != nil {
				panic(err)
			}
		}
	}()

	_, _ = joystickLeft, joystickRight
	for running {
		for event = sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch t := event.(type) {
			case *sdl.QuitEvent:
				running = false
			case *sdl.JoyAxisEvent:
				value := int16(t.Value)
				axis[t.Axis] = value
				if t.Axis == 3 || t.Axis == 4 {
					if mode == ModeManual {
						if axis[3] < 20000 && axis[3] > -20000 {
							if axis[4] < -32000 {
								joystickRight = JoystickStateUp
							} else if axis[4] > 32000 {
								joystickRight = JoystickStateDown
							} else {
								joystickRight = JoystickStateNone
							}
						} else {
							joystickRight = JoystickStateNone
						}
					}
					//fmt.Printf("right [%d ms] Which: %v \t%d %d\n",
					//              t.Timestamp, t.Which, axis[3], axis[4])
				} else if t.Axis == 0 || t.Axis == 1 {
					if mode == ModeManual {
						if axis[0] < 20000 && axis[0] > -20000 {
							if axis[1] < -32000 {
								joystickLeft = JoystickStateUp
							} else if axis[1] > 32000 {
								joystickLeft = JoystickStateDown
							} else {
								joystickLeft = JoystickStateNone
							}
						} else {
							joystickLeft = JoystickStateNone
						}
					}
					//fmt.Printf("left [%d ms] Which: %v \t%d %d\n",
					//t.Timestamp, t.Which, axis[0], axis[1])
				} else if t.Axis == 2 {
					//fmt.Printf("2 axis [%d ms] Which: %v \t%x\n",
					//      t.Timestamp, t.Which, value)
					//speed = axis[2]
					//pwm = int(100 * (float64(speed) + 32768) / 65535)
					//fmt.Printf("speed %d pwm %d\n", speed, pwm)
				}
			case *sdl.JoyBallEvent:
				fmt.Printf("[%d ms] Ball:%d\txrel:%d\tyrel:%d\n",
					t.Timestamp, t.Ball, t.XRel, t.YRel)
			case *sdl.JoyButtonEvent:
				fmt.Printf("[%d ms] Button:%d\tstate:%d\n",
					t.Timestamp, t.Button, t.State)
				if t.Button == 0 && t.State == 1 {
					switch mode {
					case ModeManual:
						mode = ModeAuto
					case ModeAuto:
						mode = ModeManual
						joystickLeft = JoystickStateNone
						joystickRight = JoystickStateNone
					}
				} else if t.Button == 1 && t.State == 1 {
					speed += .1
					if speed > .3 {
						speed = 0.1
					}
				}
			case *sdl.JoyHatEvent:
				fmt.Printf("[%d ms] Hat:%d\tvalue:%d\n",
					t.Timestamp, t.Hat, t.Value)
				if t.Value == 1 {
					// up
				} else if t.Value == 4 {
					// down
				} else if t.Value == 8 {
					// left
				} else if t.Value == 2 {
					// right
				}
			case *sdl.JoyDeviceAddedEvent:
				fmt.Println(t.Which)
				joysticks[int(t.Which)] = sdl.JoystickOpen(int(t.Which))
				if joysticks[int(t.Which)] != nil {
					fmt.Printf("Joystick %d connected\n", t.Which)
				}
			case *sdl.JoyDeviceRemovedEvent:
				if joystick := joysticks[int(t.Which)]; joystick != nil {
					joystick.Close()
				}
				fmt.Printf("Joystick %d disconnected\n", t.Which)
			default:
				fmt.Printf("Unknown event\n")
			}
		}

		sdl.Delay(16)
	}
}
