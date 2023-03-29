package imgproc

import (
	"bytes"
	"io"
	"log"
	"os"
	"syscall"
)

// #include <stdio.h>
import "C"

type Capturer struct {
	origStdout int
	out        chan []byte
	w          *os.File
}

// starts capturing stdout of a cgo call
// this needs to be called before every cgo call if you want the stdout output in go's log
func NewCapturer() *Capturer {
	c := &Capturer{}
	// Clone Stdout to origStdout.
	var err error
	c.origStdout, err = syscall.Dup(syscall.Stdout)
	if err != nil {
		log.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	c.w = w

	// Clone the pipe's writer to the actual Stdout descriptor; from this point
	// on, writes to Stdout will go to w.
	if err = syscall.Dup3(int(w.Fd()), syscall.Stdout, 0); err != nil {
		log.Fatal(err)
	}

	// Background goroutine that drains the reading end of the pipe.
	c.out = make(chan []byte)
	go func() {
		var b bytes.Buffer
		io.Copy(&b, r)
		c.out <- b.Bytes()
	}()
	return c
}

// after the cgo call, dump captured stdout bytes to log
func (c *Capturer) Dump() {
	// Cleanup
	C.fflush(nil)
	c.w.Close()
	syscall.Close(syscall.Stdout)

	// Rendezvous with the reading goroutine.
	b := <-c.out

	// Restore original Stdout.
	_ = syscall.Dup3(c.origStdout, syscall.Stdout, 0)
	syscall.Close(c.origStdout)

	log.Println(string(b))
}
