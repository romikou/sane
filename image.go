// Copyright (C) 2013 Tiago Quelhas. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sane

import (
	"fmt"
	"image"
	"image/color"
)

var (
	opaque8  = uint8(0xff)
	opaque16 = uint16(0xffff)
)

// Image is a scanned image, corresponding to one or more frames.
//
// It implements the image.Image interface.
type Image struct {
	fs [3]*Frame // multiple frames must be in RGB order
}

// Bounds returns the domain for which At returns valid pixels.
func (m *Image) Bounds() image.Rectangle {
	f := m.fs[0]
	return image.Rect(0, 0, f.Width, f.Height)
}

// ColorModel returns the Image's color model.
func (m *Image) ColorModel() color.Model {
	f := m.fs[0]
	switch {
	case f.Depth != 16 && f.Format == FrameGray:
		return color.GrayModel
	case f.Depth == 16 && f.Format == FrameGray:
		return color.Gray16Model
	case f.Depth != 16 && f.Format != FrameGray:
		return color.RGBAModel
	case f.Depth == 16 && f.Format != FrameGray:
		return color.RGBA64Model
	}
	return color.RGBAModel
}

// At returns the color of the pixel at (x, y).
func (m *Image) At(x, y int) color.Color {
	if x < 0 || x >= m.fs[0].Width || y < 0 || y >= m.fs[0].Height {
		return color.RGBA{}
	}
	if m.fs[0].Format == FrameGray {
		// grayscale
		switch m.fs[0].Depth {
		case 1:
			return color.Gray{uint8(0xFF * m.fs[0].At(x, y, 0))}
		case 8:
			return color.Gray{uint8(m.fs[0].At(x, y, 0))}
		case 16:
			return color.Gray16{m.fs[0].At(x, y, 0)}
		}
	} else {
		// color
		var r, g, b uint16
		if m.fs[0].Format == FrameRgb {
			// interleaved
			r = m.fs[0].At(x, y, 0)
			g = m.fs[0].At(x, y, 1)
			b = m.fs[0].At(x, y, 2)
		} else {
			// non-interleaved
			r = m.fs[0].At(x, y, 0)
			g = m.fs[1].At(x, y, 0)
			b = m.fs[2].At(x, y, 0)
		}
		switch m.fs[0].Depth {
		case 1:
			return color.RGBA{uint8(0xFF * r), uint8(0xFF * g), uint8(0xFF * b), opaque8}
		case 8:
			return color.RGBA{uint8(r), uint8(g), uint8(b), opaque8}
		case 16:
			return color.RGBA64{r, g, b, opaque16}
		}
	}
	return color.RGBA{} // shouldn't happen
}

func (c *Conn) loadImage() (*Image, error) {
	m := Image{}
	for {
		f, err := c.ReadFrame()
		if err != nil {
			return nil, err
		}
		switch f.Format {
		case FrameGray, FrameRgb, FrameRed:
			m.fs[0] = f
		case FrameGreen:
			m.fs[1] = f
		case FrameBlue:
			m.fs[2] = f
		default:
			return nil, fmt.Errorf("unknown frame type %d", f.Format)
		}
		if f.IsLast {
			break
		}
	}
	return &m, nil
}

// ReadImage reads an image from the connection.
func (c *Conn) ReadImage() (*Image, error) {
	defer c.Cancel()
	return c.loadImage()
}

// ReadAvailableImages reads all available image from the connection.
// This is required for example for duplex scanners like the Fujitsu
// ix500 as ReadImage only fetches one page from the scanner.
func (c *Conn) ReadAvailableImages() ([]*Image, error) {
	defer c.Cancel()

	var images = []*Image{}

	for {
		m, err := c.loadImage()
		if err != nil {
			if err == ErrEmpty && len(images) > 0 {
				// This is expected in multi-page scenarios and signals
				// there are no more pages to come.
				break
			}

			// Other errors are returned
			return nil, err
		}
		images = append(images, m)
	}

	return images, nil
}

// ContinuousRead reads all images from connection and process each image
// Useful for ADF scanners, fetch images one by one is slow
func (c *Conn) ContinuousRead(process func(m *Image) error) error {
	defer c.Cancel()

	var (
		m   *Image
		err error
	)
	// Tray can be empty, return on any error
	m, err = c.loadImage()
	if err != nil {
		return err
	}
	for {
		if err := process(m); err != nil {
			return err
		}
		m, err = c.loadImage()
		if err != nil {
			if err == ErrEmpty {
				//No more documents in tray
				break
			}
			return err
		}
	}
	return nil
}
