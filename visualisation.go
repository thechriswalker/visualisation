package main

import (
	"image"
	"image/color"
	"math"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/rasterizer"
)

const (
	spectrumHeightMultiplier = 8
)

// for accessing the [2]float64
const (
	X = 0
	Y = 1
)

type VisCache struct {
	raw      []float64
	smoothed []float64
	points   [][2]float64
}

type Visualisation struct {
	img           *image.RGBA // the image we will write to and repeatedly output
	width, height float64
	cache         []*VisCache
	numSpectrums  int // so save having to count all the time
	frame         int // current frame number
}

// SpectrumStyle slice
// note that the spectrums are drawn oldest to newest
// so the colors will be oldest to newest. i.e we want white
// at the end.
// also we cannot cache them, because we need to draw them
// as different sizes... with the exponents....
type SpectrumStyle struct {
	color     color.Color
	exponent  float64
	smoothing int
}

// notes from js.nation
// the audiocontext analyser node uses
// a smoothingTimeContstant
// min/max Decibels....
// and the getByteFrequencyData
// is put into a frequencyBinCount size array.
//

var (
	spectrumStyles []SpectrumStyle = []SpectrumStyle{
		{
			color:     color.RGBA{0x00, 0xff, 0x00, 0xff}, // 00ff00ff: green
			exponent:  1.52,
			smoothing: 5,
		},
		{
			color:     color.RGBA{0x33, 0xcc, 0xff, 0xff}, // 33ccffff: lightblue
			exponent:  1.50,
			smoothing: 5,
		},
		{
			color:     color.RGBA{0x00, 0x00, 0xff, 0xff}, // 0000ffff: blue
			exponent:  1.36,
			smoothing: 3,
		},
		{
			color:     color.RGBA{0x33, 0x33, 0x99, 0xff}, // 333399ff: indigo
			exponent:  1.33,
			smoothing: 3,
		},
		{
			color:     color.RGBA{0xff, 0x66, 0xff, 0xff}, // ff66ffff: pink
			exponent:  1.30,
			smoothing: 3,
		},
		{
			color:     color.RGBA{0xff, 0x00, 0x00, 0xff}, // ff0000ff: red
			exponent:  1.14,
			smoothing: 2,
		},
		{
			color:     color.RGBA{0xff, 0xff, 0x00, 0xff}, // ffff00ff: yellow
			exponent:  1.12,
			smoothing: 2,
		},
		{color: color.White,
			exponent:  1,
			smoothing: 1,
		},
	}
)

func NewVisualisation(c *Config) *Visualisation {
	img := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	n := len(spectrumStyles)
	v := &Visualisation{
		img:          img,
		width:        float64(c.Width),
		height:       float64(c.Height),
		cache:        make([]*VisCache, n),
		numSpectrums: n,
	}
	return v
}

// CreateFrame draws a single frame from the audio given.
func (v *Visualisation) CreateFrame(af *AudioFrame) *image.RGBA {
	// add the new audioframe
	c := canvas.New(v.width, v.height)
	ctx := canvas.NewContext(c)
	// create the new "spectrum" add it to a stack of them
	if v.frame < v.numSpectrums {
		// we need to allocate the next one.
		v.cache[v.frame] = &VisCache{
			raw:      make([]float64, len(af.freq)),
			smoothed: make([]float64, len(af.freq)),
			points:   make([][2]float64, len(af.freq)),
		}
	}
	// copy the current data into the spectrum cache
	copy(v.cache[v.frame%v.numSpectrums].raw, af.freq)

	// draw our canvas
	v.draw(ctx)
	// dump the data
	r := rasterizer.New(v.img, 1)
	c.Render(r)

	//increase the frame number after handling a frame
	v.frame++

	// return the img
	return v.img
}

func (v *Visualisation) draw(ctx *canvas.Context) {
	// first fill in black
	ctx.SetFillColor(color.Black)
	ctx.DrawPath(0, 0, canvas.Rectangle(v.width, v.height))
	halfHeight := v.height / 2
	halfWidth := v.width / 2
	radius := v.height / 4

	// now draw a path around the circle in the shape of a spectrum analyser.
	// so polar cordinates for the points based on volume at frequency.
	// and mirror the path on both sides of the circle.
	for s := 0; s < v.numSpectrums; s++ {
		// this is the number of the frame numSpectrums-1 ago + s
		x := v.frame - (v.numSpectrums - 1) + s
		if x < 0 {
			// we don't have these frames just yet we must be starting
			continue
		}
		// to draw the spectrum we must first create all the points.
		// we use the pointsCache for this to save allocation every frame
		// style like `s`

		idx := x % v.numSpectrums
		style := spectrumStyles[idx]
		cache := v.cache[idx]
		v.doSmoothing(cache, style.smoothing)
		// now create all the x/y co-ordinates.
		l := len(cache.points)
		for i := 0; i < l; i++ {
			t := math.Pi*(float64(i)/float64(l-1)) - math.Pi/2
			r := radius + math.Pow(cache.smoothed[i]*spectrumHeightMultiplier, style.exponent)

			cache.points[i] = [2]float64{
				r * math.Cos(t), // x
				r * math.Sin(t), // y
			}
		}
		// now the smoothing passes

		pts := cache.points
		// now we can make the path and draw
		p := &canvas.Path{}
		// the top of the circle (or the height of the first point above the top)
		p.MoveTo(0, pts[0][Y])
		for j := 1; j < l-2; j++ {
			p.QuadTo(
				pts[j][X], pts[j][Y],
				(pts[j][X]+pts[j+1][X])/2,
				(pts[j][Y]+pts[j+1][Y])/2,
			)
		}
		// finally the curve to the final point.
		p.QuadTo(
			pts[l-2][X],
			pts[l-2][Y],
			pts[l-1][X],
			pts[l-1][Y],
		)
		// now the other side.
		for j := 1; j < l-2; j++ {
			p.QuadTo(
				-1*pts[j][X],
				pts[j][Y],
				-1*(pts[j][X]+pts[j+1][X])/2,
				(pts[j][Y]+pts[j+1][Y])/2,
			)
		}
		p.QuadTo(
			-1*pts[l-2][X],
			pts[l-2][Y],
			-1*pts[l-1][X],
			pts[l-1][Y],
		)
		p.Close()
		// let's draw this!
		ctx.SetFillColor(style.color)
		ctx.DrawPath(halfWidth, halfHeight, p)
	}

	// then lets draw a circle in the middle
	ctx.SetFillColor(color.White)
	ctx.DrawPath(halfWidth, halfHeight, canvas.Circle(radius))
}

func (v *Visualisation) doSmoothing(cache *VisCache, margin int) {
	for i := 0; i < len(cache.raw); i++ {
		var sum, denom float64
		for j := 0; j < margin; j++ {
			if i-j < 0 || i+j > len(cache.raw)-1 {
				break
			}
			sum += cache.raw[i-j] + cache.raw[i+j]
			denom += float64(margin-j+1) * 2
		}
		cache.smoothed[i] = sum / denom
	}
}
