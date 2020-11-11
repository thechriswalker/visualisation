package main

import (
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"strconv"
)

// VideoSink is the output file, created by ffmpeg again, that will encode the
// video we pass into it (our generated visualisation) frame by frame
type VideoSink struct {
	Cmd   *exec.Cmd // ffmpeg -i <audio> -i - -f rawvideo -pix_fmt argb -s 1280x720 -r 30 -c:v libx264 <opt>
	stdin io.WriteCloser
}

// NewVideoSink creates the ffmpeg task to read in raw pixel data
// and encode according to the options.
func NewVideoSink(c *Config) (*VideoSink, error) {
	dim := fmt.Sprintf("%dx%d", c.Width, c.Height)
	args := []string{}

	// audio input file
	args = append(args, "-i", c.AudioFile)
	// stdin for video in raw rgba format.
	args = append(args,
		"-thread_queue_size", "32",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", dim,
		"-r", strconv.Itoa(c.FPS),
		"-i", "-",
	)

	// set output video codec
	args = append(args, "-c:v")
	args = append(args, c.VideoCodecAndOptions...)
	// set output audio codec
	args = append(args, "-c:a")
	args = append(args, c.AudioCodecAndOptions...)

	// set output video file (and use `-y` to overwrite)
	args = append(args, "-y", c.VideoFile)
	cmd := exec.Command(c.FFMpegPath, args...)

	// get a handle on a pipe to stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	// we need to start the process as well.
	vs := &VideoSink{
		Cmd:   cmd,
		stdin: stdin,
	}
	return vs, cmd.Start()
}

// Finish lets the sink know you are done sending frames
func (vs *VideoSink) Finish() error {
	// we are done. close the stdin pipe and let ffmpeg finish
	vs.stdin.Close()
	return vs.Cmd.Wait()
}

// SendFrame sends the data from the image to the buffer.
// this is image.RGBA as that is what we want to send to FFMPEG
// TBH as long as the format is compatible with ffmpegs `-pix_fmt`
// arg and the image type matches we can use it.
// It may be more performant to use a YUV image type.
func (vs *VideoSink) SendFrame(img *image.RGBA) error {
	// this blocks until the data is copied, so we should be OK
	// as long as the frames are processed in order.
	// From the RGBA docs:
	//  > Pix holds the image's pixels, in R, G, B, A order. The pixel at
	//  > (x, y) starts at Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*4].
	// But we will assume it's the whole thing.
	// and we will ensure we write the whole thing or fail.
	n := 0
	var i int
	var err error
	for n < len(img.Pix) {
		i, err = vs.stdin.Write(img.Pix[n:])
		n += i
		if err != nil {
			break
		}
	}
	return err
}

// turns out I didn't need this, but we will leave it...
// I forgot about cmd.StdinPipe()

// FrameBuffer for reading/writing to from a canvas to the outputstream
// this allows us to write to a buffer, have it read and then write again to the same buffer.
// we will use a mutex to easily control read/write
// basically if read is called before an image is ready we need to signal that we are ready to read
// i.e. we should write.
// if we are ready to write a frame, we must wait until the current frame is read.
// so we start in write mode (we must write the first image before it can be read.)
// a channel is not the most efficient, but it is certainly the easiest to synchronize
type FrameBuffer struct {
	size int
	r    chan []byte
	w    chan []byte
	_r   []byte // the current in progress read buffer
	_p   int    // the progress through the read buffer.
}

// NewFrameBuffer creates a new frame buffer with the given buffer size
func NewFrameBuffer(size int) *FrameBuffer {
	// we will double buffer. so we make space for 2
	r := make(chan []byte, 2)
	w := make(chan []byte, 2)

	// our buffers, both of them
	// put them both in the write channel
	w <- make([]byte, size)
	w <- make([]byte, size)

	return &FrameBuffer{size: size, r: r, w: w}
}

// WriteFrame writes into a buffer
func (fb *FrameBuffer) WriteFrame(p []byte) {
	if len(p) != fb.size {
		panic("fb.WriteFrame called with mismatched frame size.")
	}
	// take a buffer from the write queue
	b := <-fb.w
	// copy the data
	copy(b[:], p)
	// put it in the read queue
	fb.r <- b
}

// Close the buffer, no more writes will work
func (fb *FrameBuffer) Close() {
	// if we close this, frames that have been written will still
	// be processed but no more.
	close(fb.r)
	// we don't close the write channel as it complicates
	// the Read process. so for this framebuffer, it is just
	// going to silently fail to write frames written after
	// calling .Close()
}

// Read to implement io.Reader
// does this by reading from the read buffer
func (fb *FrameBuffer) Read(p []byte) (int, error) {
	l := len(p) // this is how much we want to read
	n := 0      // this is how much we have read so far
	for n < l {
		// are we continueing from a previous frame?
		if fb._r == nil {
			// nope, we finished a frame
			fb._r = <-fb.r // pick the next frame from the channel
			if fb._r == nil {
				// nil channel read. we are done
				return n, io.EOF
			}
		}
		// we should have a buffer in fb._r now.
		// we copy into p from offset n, from fb._r offset fb._p
		i := copy(p[n:], fb._r[fb._p:])
		// advance our pointers.
		fb._p += i
		n += i
		// if we read the full frame, then we much 0 it now.
		if fb._p == len(fb._r) {
			// return the buffer to the "write" queue for re-use
			fb.w <- fb._r
			fb._r = nil
			fb._p = 0
		}
	}
	// return how much we read and no error
	return n, nil
}

var _ io.Reader = (*FrameBuffer)(nil)
