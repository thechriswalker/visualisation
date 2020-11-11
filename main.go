package main

import (
	"flag"
	"log"
	"os/exec"
)

// read in an MP3
// process it to get some frequency analysis at 30fps.
// draw a circular spectrum analyser like trap nation.
// Lets assume we have ffmpeg.
// ffmpeg a give us a raw pcm stream.

// need a file system to store the file so we can get ffmpeg to load it twice.
type Config struct {
	FFMpegPath string

	// audio input config
	AudioFile string

	// video output config
	VideoFile            string
	Width                int
	Height               int
	FPS                  int
	VideoCodecAndOptions []string
	AudioCodecAndOptions []string
}

var (
	// default video will be 720p30
	defaultWidth  = 1280
	defaultHeight = 720
	defaultFPS    = 30
	// default codec options
	defaultVideoOptions = []string{"libx264", "-preset", "ultrafast", "-crf", "0"} // 264 is simple enough
	defaultAudioOptions = []string{"copy"}                                         // keep whatever the original was
)

var (
	infile  = flag.String("audio", "", "The path to an audio file for input")
	outfile = flag.String("video", "output/output.mkv", "The path to a video file for output")
)

func main() {
	flag.Parse()

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		log.Fatalln("Can't find ffmpeg in path:", err)
	}

	if *infile == "" {
		log.Fatal("Must provide an audio input file '-audio'")
	}
	if *outfile == "" {
		log.Fatal("Must provide a video output destination '-video'")
	}
	// create config
	config := &Config{
		FFMpegPath:           ffmpeg,
		AudioFile:            *infile,
		VideoFile:            *outfile,
		FPS:                  defaultFPS,
		Width:                defaultWidth,
		Height:               defaultHeight,
		VideoCodecAndOptions: defaultVideoOptions,
		AudioCodecAndOptions: defaultAudioOptions,
	}

	audio, err := NewAudioSource(config)
	if err != nil {
		panic(err)
	}

	video, err := NewVideoSink(config)
	if err != nil {
		panic(err)
	}

	vis := NewVisualisation(config)

	err = audio.StartProcessing(func(f *AudioFrame) error {
		img := vis.CreateFrame(f)
		return video.SendFrame(img)
	})
	if err != nil {
		panic(err)
	}

}
