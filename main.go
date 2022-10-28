package main

import (
	"context"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"git.sr.ht/~ghost08/photon/imgproc"
	"git.sr.ht/~ghost08/photon/lib"
	"git.sr.ht/~ghost08/photon/lib/keybindings"
	"git.sr.ht/~ghost08/photon/lib/states"

	"github.com/alecthomas/kong"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-isatty"
)

var CLI struct {
	Extractor       string       `optional:"" default:"yt-dlp --get-url %" help:"command for media link extraction (item link is substituted for %)" env:"PHOTON_EXTRACTOR"`
	VideoCmd        string       `optional:"" default:"mpv $" help:"set default command for opening the item media link in a video player (media link is substituted for %, direct item link is substituted for $, if no % or $ is provided, photon will download the data and pipe it to the stdin of the command)" env:"PHOTON_VIDEOCMD"`
	ImageCmd        string       `optional:"" default:"imv -" help:"set default command for opening the item media link in a image viewer (media link is substituted for %, direct item link is substituted for $, if no % or $ is provided, photon will download the data and pipe it to the stdin of the command)" env:"PHOTON_IMAGECMD"`
	TorrentCmd      string       `optional:"" default:"mpv %" help:"set default command for opening the item media link in a torrent downloader (media link is substituted for %, if link is a torrent file, photon will download it, and substitute the torrent file path for %)" env:"PHOTON_TORRENTCMD"`
	ArticleMode     string       `optional:"" default:"ARTICLE" enum:"ARTICLE,DESCRIPTION,CONTENT" help:"the default article view mode" env:"PHOTON_ARTICLE_MODE"`
	ArticleRenderer string       `optional:"" default:"w3m -T text/html -dump -cols 72" help:"command to render the item.Content/item.Description" env:"PHOTON_ARTICLE_RENDERER"`
	HTTPSettings    HTTPSettings `embed:""`
	DownloadPath    string       `optional:"" default:"$HOME/Downloads" help:"the default download path"`
	TerminalTitle   string       `short:"t" optional:"" help:"set the terminal title"`
	Refresh         uint         `short:"r" optional:"" default:"0" help:"set refresh interval in seconds" env:"PHOTON_REFRESH"`
	Paths           []string     `arg:"" optional:"" help:"RSS/Atom urls, config path, or - for stdin"`
}

var (
	photon          *lib.Photon
	SelectedCard    *lib.Card
	SelectedCardPos image.Point
	cb              Callbacks
	command         string
	commandFocus    bool
	redrawCh        = make(chan bool, 1024)
)

func main() {
	// defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	// args
	kong.Parse(&CLI,
		kong.Name("photon"),
		kong.Description("Fast RSS reader as light as a photon"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}))

	if isatty.IsTerminal(os.Stdout.Fd()) {
		// don't log to terminal
		log.SetOutput(io.Discard)
		os.Stdout, _ = os.Open(os.DevNull)
	} else {
		// log to redirected stdout
		log.SetOutput(os.Stdout)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if CLI.TerminalTitle != "" {
		setTerminalTitle(CLI.TerminalTitle)
	}

	if len(CLI.Paths) == 0 {
		confDir, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		defaultConf := filepath.Join(confDir, "photon", "config")
		if _, err := os.Stat(defaultConf); os.IsNotExist(err) {
			log.Fatal(err)
		}
		CLI.Paths = []string{defaultConf}
	}

	// photon
	grid := &Grid{Columns: 5}
	cb = Callbacks{grid: grid}
	var err error
	options := []lib.Option{
		lib.WithHTTPClient(CLI.HTTPSettings.Client()),
		lib.WithMediaExtractor(CLI.Extractor),
		lib.WithMediaVideoCmd(CLI.VideoCmd),
		lib.WithMediaImageCmd(CLI.ImageCmd),
		lib.WithMediaTorrentCmd(CLI.TorrentCmd),
		lib.WithDownloadPath(CLI.DownloadPath),
	}
	if err := imgproc.Init(); err != nil {
		log.Printf("INFO: error loading opencl image resizer, falling back to CPU scaling: %s", err)
	} else {
		options = append(options, lib.WithImageCache(&imgproc.Cache{}))
	}
	photon, err = lib.New(cb, CLI.Paths, options...)
	if err != nil {
		log.Fatal(err)
	}

	// tui
	tcell.SetEncodingFallback(tcell.EncodingFallbackASCII)
	s, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err = s.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer s.Fini()

	ctx, quit := WithCancel(Background())
	grid.Resize(ctx)

	go func() {
		photon.DownloadFeeds()
		redraw(true)
		if CLI.Refresh > 0 {
			for range time.Tick(time.Duration(CLI.Refresh) * time.Second) {
				photon.DownloadFeeds()
				grid.FirstChildIndex = 0
				grid.FirstChildOffset = 0
				if len(photon.VisibleCards) > 0 {
					SelectedCard = photon.VisibleCards[0]
					SelectedCardPos = image.Point{}
				}
				redraw(true)
			}
		}
	}()

	defaultKeyBindings(s, grid, &quit)

	go func() {
		for {
			ev := s.PollEvent()
			switch ev := ev.(type) {
			case *tcell.EventKey:
				if commandInput(s, ev) {
					grid.FirstChildIndex = 0
					grid.FirstChildOffset = 0
					grid.ClearCardsPosition()
					continue
				}
				photon.KeyBindings.Run(newKeyEvent(ev))
			case *tcell.EventResize:
				s.Clear()
				newCtx := Background()
				grid.Resize(newCtx)
				switch cb.State() {
				case states.Normal:
					if newCtx.Cols != ctx.Cols {
						grid.ClearImages()
						imgproc.ProcClear()
					} else {
						grid.ClearCardsPosition()
					}
				case states.Article:
					openedArticle.Clear()
				}
				ctx, quit = WithCancel(newCtx)
				ctx.Height -= 1
				redraw(true)
			}
		}
	}()

	ctx.Height -= 1
	var fullRedraw bool
	sixelScreen := &imgproc.SixelScreen{}
	for {
		// Begin synchronized update (BSU) ESC P = 1 s ESC \
		os.Stderr.Write([]byte("\033P=1s\033\\"))
		// draw main widget + status bar
		var widgetStatus richtext
		switch cb.State() {
		case states.Normal, states.Search:
			widgetStatus = grid.Draw(ctx, s, sixelScreen, fullRedraw)
			drawCommand(ctx, s)
		case states.Article:
			widgetStatus = openedArticle.Draw(ctx, s, sixelScreen)
		}
		status := photon.GetStatus()
		if utf8.RuneCountInString(status) > (ctx.Width / 2) {
			status = string([]rune(status)[:ctx.Width/2])
		}
		drawStatusBar(
			s,
			append(
				richtext{{Text: status, Style: tcell.StyleDefault}},
				widgetStatus...,
			),
		)
		// command line cursor
		if commandFocus {
			s.SetContent(len(command), int(ctx.Rows-1), ' ', nil, tcell.StyleDefault.Reverse(true))
		}
		if fullRedraw {
			s.Sync()
		} else {
			s.Show()
		}
		// draw sixels
		sixelScreen.Write(os.Stderr)
		sixelScreen.Reset()
		// end synchronized update (ESU) ESC P = 2 s ESC \
		os.Stderr.Write([]byte("\033P=2s\033\\"))
		// wait for another redraw event or quit
		select {
		case <-ctx.Done():
			return
		case fullRedraw = <-redrawCh:
		}
	}
}

func drawStatusBar(s tcell.Screen, t richtext) {
	w, h := s.Size()
	X := w - t.Len()
	Y := h - 1
	for _, to := range t {
		drawString(s, X, Y, to.Text, to.Style)
		X += len(to.Text)
	}
}

func redraw(full bool) {
	redrawCh <- full
}

// converts tcell.EventKey to keybindings.KeyEvent
func newKeyEvent(e *tcell.EventKey) keybindings.KeyEvent {
	var mod keybindings.Modifiers
	switch {
	case e.Modifiers()&tcell.ModCtrl != 0:
		mod = keybindings.ModCtrl
	case e.Modifiers()&tcell.ModShift != 0:
		mod = keybindings.ModShift
	case e.Modifiers()&tcell.ModAlt != 0:
		mod = keybindings.ModAlt
	case e.Modifiers()&tcell.ModMeta != 0:
		mod = keybindings.ModSuper
	}

	var r rune
	switch e.Key() {
	case tcell.KeyBackspace:
		return keybindings.KeyEvent{Key: '\u0008'}
	case tcell.KeyTab:
		return keybindings.KeyEvent{Key: '\t'}
	case tcell.KeyEsc:
		return keybindings.KeyEvent{Key: '\u00b1'}
	case tcell.KeyEnter:
		return keybindings.KeyEvent{Key: '\n'}
	case tcell.KeyRune:
		if unicode.IsUpper(e.Rune()) {
			mod = keybindings.ModShift
		}
		r = unicode.ToLower(e.Rune())
		return keybindings.KeyEvent{Key: r, Modifiers: mod}
	case tcell.KeyLeft:
		return keybindings.KeyEvent{Key: 37}
	case tcell.KeyUp:
		return keybindings.KeyEvent{Key: 38}
	case tcell.KeyRight:
		return keybindings.KeyEvent{Key: 39}
	case tcell.KeyDown:
		return keybindings.KeyEvent{Key: 40}
	default:
		s, ok := tcell.KeyNames[e.Key()]
		if ok && strings.HasPrefix(s, "Ctrl-") {
			s = s[5:]
			r, _ = utf8.DecodeLastRuneInString(s)
			r = unicode.ToLower(r)
		}
		return keybindings.KeyEvent{Key: r, Modifiers: mod}
	}
}

func defaultKeyBindings(s tcell.Screen, grid *Grid, quit *context.CancelFunc) {
	// NormalState
	photon.KeyBindings.Add(states.Normal, "q", func() error {
		if quit != nil {
			q := *quit
			quit = nil
			q()
		}
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "<enter>", func() error {
		SelectedCard.OpenArticle()
		if openedArticle != nil {
			openedArticle.Mode = articleModeFromString(CLI.ArticleMode)
		}
		grid.ClearCardsPosition()
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "r", func() error {
		photon.DownloadFeeds()
		grid.FirstChildIndex = 0
		grid.FirstChildOffset = 0
		if len(photon.VisibleCards) > 0 {
			SelectedCard = photon.VisibleCards[0]
			SelectedCardPos = image.Point{}
		}
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "p", func() error {
		SelectedCard.RunMedia()
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "o", func() error {
		SelectedCard.OpenBrowser()
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "<esc>", func() error {
		if command == "" {
			return nil
		}
		command = ""
		commandFocus = false
		photon.SearchQuery("")
		grid.ClearCardsPosition()
		grid.FirstChildIndex = 0
		grid.FirstChildOffset = 0
		if len(photon.VisibleCards) > 0 {
			SelectedCard = photon.VisibleCards[0]
		}
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "=", func() error {
		grid.Columns++
		grid.ClearImages()
		grid.Resize(Background())
		imgproc.ProcClear()
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "-", func() error {
		if grid.Columns == 1 {
			return nil
		}
		grid.Columns--
		grid.ClearImages()
		grid.Resize(Background())
		imgproc.ProcClear()
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "/", func() error {
		if command != "" && !commandFocus {
			commandFocus = true
			redraw(false)
			return nil
		}
		command = "/"
		commandFocus = true
		redraw(false)
		return nil
	})
	// copy item link
	photon.KeyBindings.Add(states.Normal, "yy", func() error {
		if SelectedCard == nil {
			return nil
		}
		osc52(SelectedCard.Item.Link)
		return nil
	})
	// copy item image
	/*
		photon.KeyBindings.Add(states.Normal, "yi", func() error {
			if SelectedCard == nil {
				return nil
			}
			if SelectedCard.ItemImage == nil {
				return nil
			}
			if !clip {
				return nil
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, SelectedCard.ItemImage.(image.Image)); err != nil {
				return fmt.Errorf("encoding image: %w", err)
			}
			clipboard.Write(clipboard.FmtImage, buf.Bytes())
			return nil
		})
	*/
	// download media
	photon.KeyBindings.Add(states.Normal, "dm", func() error {
		SelectedCard.DownloadMedia()
		return nil
	})
	// download link
	photon.KeyBindings.Add(states.Normal, "dl", func() error {
		SelectedCard.DownloadLink()
		return nil
	})
	// download image
	photon.KeyBindings.Add(states.Normal, "di", func() error {
		SelectedCard.DownloadImage()
		return nil
	})
	// move selectedCard
	photon.KeyBindings.Add(states.Normal, "h", func() error {
		grid.SelectedChildMoveLeft()
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "l", func() error {
		grid.SelectedChildMoveRight()
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "j", func() error {
		grid.SelectedChildMoveDown()
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "k", func() error {
		grid.SelectedChildMoveUp()
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "<ctrl>d", func() error {
		_, h := s.Size()
		grid.Scroll((h - 1) / 2)
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "<ctrl>u", func() error {
		_, h := s.Size()
		grid.Scroll(-(h - 1) / 2)
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "<ctrl>f", func() error {
		_, h := s.Size()
		grid.Scroll(h - 1)
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "<ctrl>b", func() error {
		_, h := s.Size()
		grid.Scroll(1 - h)
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "gg", func() error {
		if grid.FirstChildIndex == 0 && grid.FirstChildOffset == 0 {
			return nil
		}
		grid.FirstChildIndex = 0
		grid.FirstChildOffset = 0
		SelectedCardPos.Y = 0
		selectedCardIndex := SelectedCardPos.Y*grid.Columns + SelectedCardPos.X
		if selectedCardIndex < len(photon.VisibleCards) {
			SelectedCard = photon.VisibleCards[selectedCardIndex]
		}
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "<shift>g", func() error {
		grid.FirstChildIndex = len(photon.VisibleCards) - grid.RowsCount
		SelectedCardPos.Y = len(photon.VisibleCards)/grid.Columns - 1
		selectedCardIndex := SelectedCardPos.Y*grid.Columns + SelectedCardPos.X
		if selectedCardIndex < len(photon.VisibleCards) {
			SelectedCard = photon.VisibleCards[selectedCardIndex]
		}
		redraw(true)
		return nil
	})

	// SearchState
	photon.KeyBindings.Add(states.Search, "<enter>", func() error {
		commandFocus = false
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Search, "<esc>", func() error {
		command = ""
		commandFocus = false
		photon.SearchQuery("")
		grid.ClearCardsPosition()
		grid.FirstChildIndex = 0
		grid.FirstChildOffset = 0
		if len(photon.VisibleCards) > 0 {
			SelectedCard = photon.VisibleCards[0]
			SelectedCardPos = image.Point{}
		}
		redraw(true)
		return nil
	})

	// ArticleState
	photon.KeyBindings.Add(states.Article, "<esc>", func() error {
		openedArticle = nil
		photon.OpenedArticle = nil
		s.Clear()
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "q", func() error {
		openedArticle = nil
		photon.OpenedArticle = nil
		s.Clear()
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Normal, "yy", func() error {
		// copy article link
		if openedArticle == nil {
			return nil
		}
		osc52(openedArticle.Card.Item.Link)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "o", func() error {
		openedArticle.Card.OpenBrowser()
		return nil
	})
	photon.KeyBindings.Add(states.Article, "m", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.ToggleMode()
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "j", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.Scroll(1)
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "k", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.Scroll(-1)
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "gg", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.scrollOffset = 0
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "<shift>g", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.scrollOffset = openedArticle.lastLine - len(openedArticle.contentLines)
		redraw(false)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "<ctrl>d", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.Scroll((openedArticle.lastLine - openedArticle.scrollOffset) / 2)
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "<ctrl>u", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.Scroll(-(openedArticle.lastLine - openedArticle.scrollOffset) / 2)
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "<ctrl>f", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.Scroll(openedArticle.lastLine - openedArticle.scrollOffset)
		redraw(true)
		return nil
	})
	photon.KeyBindings.Add(states.Article, "<ctrl>b", func() error {
		if openedArticle == nil {
			return nil
		}
		openedArticle.Scroll(-(openedArticle.lastLine - openedArticle.scrollOffset))
		redraw(true)
		return nil
	})
}

func setTerminalTitle(title string) {
	fmt.Fprintf(os.Stdout, "\033]2;%s\007", title)
}
