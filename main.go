// Copyright 2024 The AS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/signal"
	"syscall"
	"time"

	. "github.com/pointlander/matrix"

	"github.com/veandco/go-sdl2/sdl"
	"go.bug.st/serial"
)

var joysticks = make(map[int]*sdl.Joystick)

type (
	// JoystickState is the state of a joystick
	JoystickState uint
	// Mode is the operating mode of the robot
	Mode uint
	// Camera is a camera
	TypeCamera uint
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
	Frame image.Image
	DCT   [][]float64
	Query Matrix
	Key   Matrix
	Value Matrix
}

func main() {
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

	camera := NewV4LCamera()
	go camera.Start("/dev/video0")
	go func() {
		select {
		case img := <-camera.Images:
			_ = img
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
		previousLeft, previousRight := 0.0, 0.0
		for running {
			time.Sleep(300 * time.Millisecond)

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

			if leftSpeed == previousLeft && rightSpeed == previousRight {
				continue
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
			previousLeft, previousRight = leftSpeed, rightSpeed
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
