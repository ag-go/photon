package main

import (
	"context"
	"fmt"
	"time"
	"unsafe"
)

func Background() Context {
	ws, err := GetWinSize()
	if err != nil {
		panic(err)
	}
	return Context{
		WinSize:     ws,
		Width:       int(ws.Cols),
		Height:      int(ws.Rows),
		XCellPixels: int(ws.XPixel) / int(ws.Cols),
		YCellPixels: int(ws.YPixel) / int(ws.Rows),
	}
}

func WithCancel(ctx Context) (Context, context.CancelFunc) {
	ret := ctx
	ret.cancelChan = make(chan struct{})
	return ret, func() {
		close(ret.cancelChan)
	}
}

type Context struct {
	WinSize
	X, Y                     int
	Width, Height            int
	YCellPixels, XCellPixels int
	cancelChan               chan struct{}
}

func (ctx Context) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (ctx Context) Done() <-chan struct{} {
	return ctx.cancelChan
}

func (ctx Context) Err() error {
	return nil
}

func (ctx Context) Value(key interface{}) interface{} {
	return nil
}

type WinSize struct {
	Rows   int16 /* rows, in characters */
	Cols   int16 /* columns, in characters */
	XPixel int16 /* horizontal size, pixels */
	YPixel int16 /* vertical size, pixels */
}

func GetWinSize() (sz WinSize, err error) {
	// TIOCGWINSZ syscall
	for fd := uintptr(0); fd < 3; fd++ {
		if err = ioctl(fd, tiocgwinsz, uintptr(unsafe.Pointer(&sz))); err == nil && sz.XPixel != 0 && sz.YPixel != 0 {
			return
		}
	}
	// if pixels are 0, try CSI 14
	if sz.XPixel == 0 || sz.YPixel == 0 {
		fmt.Printf("\033[18t")
		fmt.Scanf("\xb1[%d;%dt", &sz.Rows, &sz.Cols)
		// get terminal resolution
		fmt.Printf("\033[14t")
		fmt.Scanf("\033[4;%d;%dt", &sz.YPixel, &sz.XPixel)
	}
	if sz.XPixel == 0 || sz.YPixel == 0 {
		return sz, fmt.Errorf("can't get terminal pixel resolution")
	}
	return sz, nil
}
