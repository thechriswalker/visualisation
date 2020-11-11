package main

import (
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"strconv"

	"github.com/mjibson/go-dsp/fft"
)

// AudioSource generates the samples we will use to create our visualisation
// again we will leverage ffmpeg to create the samples from the source codec
// We will attach a function to be called on every new sample that comes in
type AudioSource struct {
	Cmd             *exec.Cmd // ffmpeg -i <audio> -c:a raw -o -
	samplesPerFrame int       // 44.1Khz / FPS - this must be exact or sync will break. 30FPS works.
	stdout          io.ReadCloser
}

// NewAudioSource creates and reads the audio source
func NewAudioSource(c *Config) (*AudioSource, error) {
	// create the command and start it, but don't read from the stdout yet.
	// not until we attach the listener
	// should we do the spectrum analysis here? or raw samples.
	// we need to support time-based analysis or just frequency
	// based. I only want frequency, but maybe the "Sample"
	// type should actually be a type with methods for "TimeDomainAnalysis"
	// and "FrequencyDomainAnalysis" and we just call whichever one...
	// that way I can implement the FrequencyDomainAnalysis first.
	// but first.

	// we can
	cmd := exec.Command(c.FFMpegPath,
		"-i", c.AudioFile, //our audio file
		"-vn",                             // no video
		"-ar", strconv.Itoa(samplingRate), // get sampling rate
		"-ac", "1", //mono
		"-f", "f64be", // raw f64 output
		"-c:a", "pcm_f64be", // we can get ffmpeg to output float64 data!
		"-", // output to stdout
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	as := &AudioSource{
		Cmd:             cmd,
		samplesPerFrame: samplingRate / c.FPS,
		stdout:          stdout,
	}

	return as, cmd.Start()
}

const (
	samplingRate = 44_100 // 44.1khz sampling
)

// StartProcessing the audio
func (as *AudioSource) StartProcessing(onFrame func(ss *AudioFrame) error) error {
	// start command, read stdout
	// we output float64s, so I hope they are smooth enough!
	// We read `samplesPerFrame` samples at a time for the frame.

	// a buffer needs to be samplesetsize * bytes per sample (8!)
	// it's only mono so just one channels worth
	buf := make([]byte, as.samplesPerFrame*8) // assuming 16bit samples

	// now we read,
	// turn into float64s
	// push out the samples.
	frame := &AudioFrame{
		data:           make([]float64, as.samplesPerFrame),
		freq:           make([]float64, as.samplesPerFrame),
		windowFunction: windowFunctions["hamming"],
	}

	for {
		_, err := io.ReadFull(as.stdout, buf)
		if err != nil {
			// we are done!
			return as.Cmd.Wait()
		}
		// fill the frame
		for i := 0; i < as.samplesPerFrame; i++ {
			// read the data as a uint64, and then convert to a float64
			frame.data[i] = math.Float64frombits(binary.BigEndian.Uint64(buf[i*8 : i*8+8]))
		}
		// now process the frame.
		frame.runFrequencyAnalysis()
		// NB we will reuse this frame next time, so
		// it doesn't belong to the onFrame func and
		// should not be considered safe after that function returns
		if err := onFrame(frame); err != nil {
			return err
		}
	}
}

// these are the 3 most common.
var windowFunctions = map[string]func(i, s int) float64{
	"rectangle": func(i, s int) float64 {
		return 1
	},
	"hamming": func(i, s int) float64 {
		return 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(s-1))
	},
	"hann": func(i, s int) float64 {
		return 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(s-1)))
	},
}

// AudioFrame is a group of samples that represent the music at that slice of time
type AudioFrame struct {
	data           []float64
	freq           []float64
	windowFunction func(i, s int) float64
}

// the frequency analysis transform
// ONLY CALL THIS ONCE PER DATA
func (af *AudioFrame) runFrequencyAnalysis() {
	// convert the data to freqpoints
	// first step is the window function.
	s := len(af.data)
	for i := 0; i < s; i++ {
		af.data[i] = af.data[i] * af.windowFunction(i, s)
	}
	// we really want a power of 2 samples per frame
	// meaning we might need to grab more samples
	// and "smooth" over our time period... sounds complex.
	// lets just take the performance hit and work with our frame counts
	ft := fft.FFTReal(af.data)
	// and now convert the fft data into the volumes at grequency band
	for i := 0; i < s; i++ {
		af.freq[i] = math.Sqrt(real(ft[i])*real(ft[i])+imag(ft[i])*imag(ft[i])) * 100 / float64(s)
	}
}
