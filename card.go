package main

import (
	"fmt"
	"image"
	"io"
	"strings"
	"time"

	"git.sr.ht/~ghost08/photont/lib"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	htime "github.com/sbani/go-humanizer/time"
)

var cards = make(map[*lib.Card]*Card)

func getCard(card *lib.Card) *Card {
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
	headerHeight  = 4
	selectedColor = tcell.ColorGray
)

type Card struct {
	*lib.Card
	sixelData         *Sixel
	scaledImageBounds image.Rectangle
	//isOnScreen        func(*lib.Card)
	previousImagePos image.Point
	previousSelected bool
}

func drawLine(s tcell.Screen, X, Y, maxWidth int, text string, style tcell.Style) (width int) {
	var x int
	for _, c := range text {
		if c == '\n' {
			return
		}
		if x > maxWidth {
			return
		}
		var comb []rune
		w := runewidth.RuneWidth(c)
		width += w
		if w == 0 {
			comb = []rune{c}
			c = ' '
			w = 1
		}

		s.SetContent(x+X, Y, c, comb, style)
		x += w
	}
	return
}

func drawLinesWordwrap(s tcell.Screen, X, Y, maxWidth, maxLines int, text string, style tcell.Style) {
	var x, y int
	var word strings.Builder
	var wordLength int
	for _, c := range text {
		if c != ' ' && c != '\n' {
			word.WriteRune(c)
			wordLength += runewidth.RuneWidth(c)
			continue
		}
		for wordLength > maxWidth && y < maxLines {
			w := drawLine(s, x+X, y+Y, maxWidth-x, word.String(), style)
			wordRest := word.String()[w:]
			word.Reset()
			word.WriteString(wordRest)
			wordLength -= w
			y++
			x = 0
		}
		if y >= maxLines {
			break
		}
		if x+wordLength > maxWidth {
			y++
			x = 0
		}
		if c == '\n' || x+wordLength == maxWidth {
			drawString(s, x+X, y+Y, word.String(), style)
			word.Reset()
			wordLength = 0
			y++
			x = 0
			continue
		}
		if y >= maxLines {
			break
		}
		x += drawString(s, x+X, y+Y, word.String()+" ", style)
		word.Reset()
		wordLength = 0
	}
	if wordLength == 0 {
		return
	}
	if x+wordLength > maxWidth {
		y++
		x = 0
	}
	if y >= maxLines {
		return
	}
	drawString(s, x+X, y+Y, word.String(), style)
}

func drawString(s tcell.Screen, x, y int, text string, style tcell.Style) (width int) {
	for _, c := range text {
		var comb []rune
		w := runewidth.RuneWidth(c)
		width += w
		if w == 0 {
			comb = []rune{c}
			c = ' '
			w = 1
		}

		s.SetContent(x, y, c, comb, style)
		x += w
	}
	return
}

func (c *Card) Draw(ctx Context, s tcell.Screen, w io.Writer) {
	imageWidthInCells := c.scaledImageBounds.Dx() / ctx.XCellPixels()
	offset := (ctx.Width - imageWidthInCells) / 2
	newImagePos := image.Point{ctx.X + 1 + offset, ctx.Y + 1}
	selected := c.Card == photon.SelectedCard
	if c.previousImagePos.Eq(newImagePos) && selected == c.previousSelected {
		return
	}
	style := tcell.StyleDefault
	if selected {
		style = tcell.StyleDefault.Background(selectedColor)
	}
	if c.Item.Image == nil {
		for x := ctx.X; x < ctx.Width+ctx.X; x++ {
			for y := ctx.Y; y < ctx.Height+ctx.Y; y++ {
				s.SetContent(x, y, ' ', nil, style)
			}
		}
		drawLinesWordwrap(s, ctx.X+1, ctx.Y, ctx.Width-3, 2, c.Item.Title, style.Bold(true))
		drawLine(s, ctx.X+1, ctx.Y+2, ctx.Width-3, c.Feed.Title, style.Italic(true))
		drawLine(s, ctx.X+1, ctx.Y+3, ctx.Width-3, htime.Difference(time.Now(), *c.Item.PublishedParsed), style.Italic(true))
		drawLinesWordwrap(s, ctx.X+1, ctx.Y+headerHeight+1, ctx.Width-3, ctx.Height-headerHeight-2, c.Item.Description, style)
		return
	}

	//header
	for x := ctx.X; x < ctx.Width+ctx.X; x++ {
		for y := ctx.Height - headerHeight + ctx.Y; y < ctx.Height+ctx.Y; y++ {
			s.SetContent(x, y, ' ', nil, style)
		}
	}
	drawLinesWordwrap(s, ctx.X+1, ctx.Height-headerHeight+ctx.Y, ctx.Width-3, 2, c.Item.Title, style.Bold(true))
	drawLine(s, ctx.X+1, ctx.Height-headerHeight+ctx.Y+2, ctx.Width-3, c.Feed.Title, style.Italic(true))
	drawLine(s, ctx.X+1, ctx.Height-headerHeight+ctx.Y+3, ctx.Width-3, htime.Difference(time.Now(), *c.Item.PublishedParsed), style.Italic(true))

	if c.DownloadImage(ctx, s) {
		c.previousImagePos = image.Point{-2, -2}
		c.swapImageRegion(ctx, s)
		return
	}
	if c.sixelData == nil {
		c.previousImagePos = image.Point{-2, -2}
		c.swapImageRegion(ctx, s)
		return
	}
	if !c.previousImagePos.Eq(image.Point{-1, -1}) {
		c.swapImageRegion(ctx, s)
	}
	c.previousImagePos = newImagePos
	c.previousSelected = selected
	switch {
	case newImagePos.Y < 0:
		//if the image upper left corner is outside of the screen leave some upper sixel rows
		fmt.Fprintf(w, "\033[0;%dH", newImagePos.X) //set cursor to x, 0
		leaveRows := int((ctx.YCellPixels()*(-newImagePos.Y))/6) + 4
		c.sixelData.WriteLeaveUpper(w, leaveRows)
	case ctx.YCellPixels()*newImagePos.Y+c.scaledImageBounds.Dy() > int(ctx.YPixel):
		//if the image lover pars is outside of the screen leave some lower sixel rows
		fmt.Fprintf(w, "\033[%d;%dH", newImagePos.Y, newImagePos.X) //set cursor to x, y
		leaveRows := ((ctx.YCellPixels()*newImagePos.Y+c.scaledImageBounds.Dy())-int(ctx.YPixel))/6 + 2
		c.sixelData.WriteLeaveLower(w, leaveRows)
	default:
		fmt.Fprintf(w, "\033[%d;%dH", newImagePos.Y, newImagePos.X) //set cursor to x, y
		c.sixelData.Write(w)
	}
}

func (c *Card) swapImageRegion(ctx Context, s tcell.Screen) {
	selected := c.Card == photon.SelectedCard
	style := tcell.StyleDefault
	if selected {
		style = tcell.StyleDefault.Background(selectedColor)
	}
	for x := ctx.X; x < ctx.Width+ctx.X; x++ {
		for y := ctx.Y; y < ctx.Height-headerHeight+ctx.Y; y++ {
			r := '\u2800'
			c, _, _, _ := s.GetContent(x, y)
			if c == r {
				r = '\u2007'
			}
			s.SetContent(x, y, r, nil, style)
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
	targetWidth := ctx.Width * ctx.XCellPixels()
	targetHeight := (ctx.Height - headerHeight) * ctx.YCellPixels()
	imageProc(
		c,
		c.ItemImage,
		targetWidth,
		targetHeight,
		func(b image.Rectangle, s *Sixel) {
			c.scaledImageBounds, c.sixelData = b, s
			//if c.isOnScreen(c.Card) {
			redraw(false)
			//}
		},
	)
}

func (c *Card) ClearImage() {
	c.sixelData = nil
}
