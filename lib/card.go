package lib

import (
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"git.sr.ht/~ghost08/photon/imgproc"
	"git.sr.ht/~ghost08/photon/lib/events"
	"git.sr.ht/~ghost08/photon/lib/media"
	"github.com/kennygrant/sanitize"
	"github.com/mmcdole/gofeed"
	"github.com/skratchdot/open-golang/open"
)

type Card struct {
	photon     *Photon
	Item       *gofeed.Item
	ItemImage  imgproc.ImageResizer
	Feed       *gofeed.Feed
	FeedImage  imgproc.ImageResizer
	Article    *Article
	Media      *media.Media
	Foreground int
	Background int
}

type Cards []*Card

func (cards Cards) Len() int {
	return len(cards)
}

func (cards Cards) Less(i, k int) bool {
	return cards[i].Item.PublishedParsed.After(*cards[k].Item.PublishedParsed)
}

func (cards Cards) Swap(i, k int) {
	cards[i], cards[k] = cards[k], cards[i]
}

func (card *Card) SaveImage() func(image.Image) {
	return func(img image.Image) {
		card.ItemImage = imgproc.NewImageResizer(img)
		card.photon.cb.Redraw()
	}
}

func (card *Card) OpenArticle() {
	if card == nil {
		return
	}
	if card.Article == nil {
		article, err := newArticle(card, card.photon.httpClient)
		if err != nil {
			log.Println("ERROR: scraping link:", err)
			return
		}
		card.Article = article
		if len(card.Article.TextContent) < len(card.Item.Description) {
			card.Article.TextContent = card.Item.Description
		}
	}
	card.photon.OpenedArticle = card.Article
	card.photon.cb.ArticleChanged(card.photon.OpenedArticle)
	events.Emit(&events.ArticleOpened{
		Link: card.Item.Link,
		Card: newCardFunc(card),
	})
	if card.photon.OpenedArticle.Image != "" {
		card.photon.ImgDownloader.Download(
			card.photon.OpenedArticle.Image,
			func(i interface{}) {
				if card.photon.OpenedArticle == nil {
					return
				}
				card.photon.OpenedArticle.TopImage = imgproc.NewImageResizer(i)
				card.photon.cb.Redraw()
			},
		)
	}
	card.photon.cb.Redraw()
}

func (card *Card) GetMedia() (*media.Media, error) {
	if card == nil {
		return nil, nil
	}
	if card.Media == nil || len(card.Media.Links) == 0 {
		m, err := card.photon.mediaExtractor.NewMedia(card.Item.Link)
		if err != nil {
			return nil, err
		}
		card.Media = m
	}
	return card.Media, nil
}

func (card *Card) RunMedia() {
	if card == nil {
		return
	}
	events.Emit(&events.RunMediaStart{
		Link: card.Item.Link,
		Card: newCardFunc(card),
	})
	card.photon.SetStatusWithSpinner(fmt.Sprintf("Play \u25B6 %s", card.Item.Title))
	go func() {
		var err error
		defer func() {
			events.Emit(&events.RunMediaEnd{
				Link: card.Item.Link,
				Card: newCardFunc(card),
			})
			if err == nil {
				card.photon.StatusWithTimeout(
					fmt.Sprintf("Stop \u25AA %s", card.Item.Title),
					time.Second*5,
				)
			}
		}()
		_, err = card.GetMedia()
		if err != nil {
			log.Println("ERROR: extracting media link:", err)
			card.photon.StatusWithTimeout(
				fmt.Sprintf("ERROR: extracting media link: %s", err),
				time.Second*3,
			)
			return
		}
		card.Media.Run()
	}()
}

func (card *Card) DownloadMedia() {
	if card == nil {
		return
	}
	go func() {
		log.Println("INFO: downloading media for:", card.Item.Link)
		m, err := card.GetMedia()
		if err != nil {
			log.Println("ERROR: extracting media link:", err)
			return
		}
		if err := card.downloadLinks(card.Item.Title, m.Links); err != nil {
			log.Println("ERROR: downloading media:", err)
			return
		}
	}()
}

func (card *Card) DownloadLink() {
	if card == nil {
		return
	}
	go func() {
		if err := card.downloadLinks(card.Item.Title, []string{card.Item.Link}); err != nil {
			log.Println("ERROR: downloading link:", err)
			return
		}
	}()
}

func (card *Card) DownloadImage() {
	if card == nil || card.Item == nil || card.Item.Image == nil {
		return
	}
	go func() {
		if err := card.downloadLinks(card.Item.Title, []string{card.Item.Image.URL}); err != nil {
			log.Println("ERROR: downloading image:", err)
			return
		}
	}()
}

func (card *Card) downloadLinks(name string, links []string) error {
	// get download path
	downloadPath := card.photon.downloadPath
	if strings.Contains(downloadPath, "$HOME") {
		usr, err := user.Current()
		if err != nil {
			return err
		}
		downloadPath = strings.ReplaceAll(downloadPath, "$HOME", usr.HomeDir)
	}
	// create download path
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		if err := os.MkdirAll(downloadPath, 0x755); err != nil {
			return err
		}
	}
	for _, link := range links {
		// get response
		req, err := http.NewRequest(http.MethodGet, link, nil)
		if err != nil {
			return err
		}
		client := card.photon.httpClient
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var bodyReader io.Reader = resp.Body
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType, bodyReader, err = recycleReader(resp.Body)
		}
		if err != nil {
			return err
		}
		exts := extensionByType(contentType)
		if len(exts) > 0 {
			name += "." + exts[0]
		}
		// write data to file
		f, err := os.Create(filepath.Join(downloadPath, sanitize.Name(name)))
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, bodyReader); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		log.Printf("INFO: downloaded link %s to %s", link, filepath.Join(downloadPath, name))
	}
	return nil
}

func (card *Card) OpenBrowser() {
	if card == nil {
		return
	}
	open.Start(card.Item.Link)
	events.Emit(&events.LinkOpened{
		Link: card.Item.Link,
		Card: newCardFunc(card),
	})
}
