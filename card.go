package main

import (
	"fmt"
	"image"
	"io"

	"git.sr.ht/~ghost08/libphoton"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

var cards = make(map[*libphoton.Card]*Card)

func getCard(card *libphoton.Card) *Card {
	c, ok := cards[card]
	if !ok {
		c = &Card{
			Card: card,
		}
		cards[card] = c
	}
	return c
}

const (
	headerHeight  = 2
	selectedColor = tcell.ColorGray
)

type Card struct {
	*libphoton.Card
	selected          bool
	sixelData         []byte
	scaledImageBounds image.Rectangle
	//isOnScreen        func(*libphoton.Card)
	previousImagePos image.Point
	previousSelected bool
}

func drawLines(s tcell.Screen, X, Y, maxWidth, maxLines int, text string, style tcell.Style) {
	var x, y int
	for _, c := range text {
		if c == '\n' {
			y++
			x = 0
			continue
		}
		if x > maxWidth {
			y++
			x = 0
			if y >= maxLines {
				break
			}
		}
		var comb []rune
		w := runewidth.RuneWidth(c)
		if w == 0 {
			comb = []rune{c}
			c = ' '
			w = 1
		}

		s.SetContent(x+X, y+Y, c, comb, style)
		x += w
	}
}

func (c *Card) Draw(ctx Context, s tcell.Screen, w io.Writer) {
	background := tcell.ColorBlack
	if c.selected {
		background = selectedColor
	}
	if c.Item.Image == nil {
		for x := ctx.X; x < ctx.Width+ctx.X; x++ {
			for y := ctx.Y; y < ctx.Height+ctx.Y; y++ {
				s.SetContent(x, y, ' ', nil, tcell.StyleDefault.Background(background))
				ctx.SetRune(x, y, ' ')
			}
		}
		drawLines(
			s,
			ctx.X+1,
			ctx.Y,
			ctx.Width-3,
			headerHeight,
			c.Item.Title,
			tcell.StyleDefault.Background(background).Bold(true),
		)
		drawLines(
			s,
			ctx.X+1,
			ctx.Y+headerHeight,
			ctx.Width-3,
			ctx.Height-headerHeight,
			c.Item.Description,
			tcell.StyleDefault.Background(background),
		)
		return
	}

	//header
	for x := ctx.X; x < ctx.Width+ctx.X; x++ {
		for y := ctx.Height - headerHeight + ctx.Y; y < ctx.Height+ctx.Y; y++ {
			s.SetContent(x, y, ' ', nil, tcell.StyleDefault.Background(background))
			ctx.SetRune(x, y, ' ')
		}
	}
	drawLines(
		s,
		ctx.X+1,
		ctx.Height-headerHeight+ctx.Y,
		ctx.Width-3,
		headerHeight,
		c.Item.Title,
		tcell.StyleDefault.Background(background).Bold(true),
	)

	if c.DownloadImage(ctx, s) {
		c.previousImagePos = image.Point{-1, -1}
		c.swapImageRegion(ctx, s)
		return
	}
	if c.sixelData == nil {
		c.previousImagePos = image.Point{-1, -1}
		c.swapImageRegion(ctx, s)
		return
	}
	imgHeight := c.scaledImageBounds.Dy()
	if int(ctx.YPixel)/int(ctx.Rows)*(ctx.Y+1)+imgHeight > int(ctx.YPixel) {
		c.previousImagePos = image.Point{-1, -1}
		c.swapImageRegion(ctx, s)
		return
	}
	if ctx.Y+1 < 0 {
		c.previousImagePos = image.Point{-1, -1}
		c.swapImageRegion(ctx, s)
		return
	}
	imageWidthInCells := c.scaledImageBounds.Dx() / int(ctx.XPixel/ctx.Cols)
	offset := (ctx.Width - imageWidthInCells) / 2
	newImagePos := image.Point{ctx.X + 1 + offset, ctx.Y + 1}
	if c.previousImagePos.Eq(newImagePos) && c.selected == c.previousSelected {
		return
	}
	if !c.previousImagePos.Eq(image.Point{-1, -1}) {
		c.swapImageRegion(ctx, s)
	}
	c.previousImagePos = newImagePos
	c.previousSelected = c.selected
	fmt.Fprintf(w, "\033[%d;%dH", newImagePos.Y, newImagePos.X) //set cursor to x, y
	w.Write(c.sixelData)
}

func (c *Card) fillImageRegion(ctx Context, s tcell.Screen, r rune) {
	background := tcell.ColorBlack
	if c.selected {
		background = selectedColor
	}
	for x := ctx.X; x < ctx.Width+ctx.X; x++ {
		for y := ctx.Y; y < ctx.Height-headerHeight+ctx.Y; y++ {
			s.SetContent(x, y, r, nil, tcell.StyleDefault.Background(background))
			ctx.SetRune(x, y, r)
		}
	}
}

func (c *Card) swapImageRegion(ctx Context, s tcell.Screen) {
	background := tcell.ColorBlack
	if c.selected {
		background = selectedColor
	}
	for x := ctx.X; x < ctx.Width+ctx.X; x++ {
		for y := ctx.Y; y < ctx.Height-headerHeight+ctx.Y; y++ {
			r := '\u2800'
			if ctx.GetRune(x, y) == r {
				r = '\u2007'
			}
			s.SetContent(x, y, r, nil, tcell.StyleDefault.Background(background))
			ctx.SetRune(x, y, r)
		}
	}
}

func (c *Card) DownloadImage(ctx Context, s tcell.Screen) bool {
	if c.ItemImage != nil || c.Item.Image == nil {
		c.makeSixel(ctx, s)
		return false
	}
	photon.ImgDownloader.Download(
		c.Item.Image.URL,
		func(img image.Image) {
			c.ItemImage = img
			c.makeSixel(ctx, s)
		},
	)
	return true
}

func (c *Card) makeSixel(ctx Context, s tcell.Screen) {
	if c.sixelData != nil || c.ItemImage == nil {
		return
	}
	targetWidth := ctx.Width * int(ctx.XPixel) / int(ctx.Cols)
	targetHeight := (ctx.Height - headerHeight) * int(ctx.YPixel) / int(ctx.Rows)
	imageProc(
		c,
		c.ItemImage,
		targetWidth,
		targetHeight,
		func(b image.Rectangle, sd []byte) {
			c.scaledImageBounds, c.sixelData = b, sd
			//if c.isOnScreen(c.Card) {
			redraw(false)
			//}
		},
	)
}

func (c *Card) ClearImage() {
	c.sixelData = nil
}

func (c *Card) Select() {
	if c == nil {
		return
	}
	c.selected = true
}

func (c *Card) Unselect() {
	if c == nil {
		return
	}
	c.selected = false
}
