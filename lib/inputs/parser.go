package inputs

import (
	"errors"
	"fmt"
	"io"
)

// Parse parses the photon config input, and retunts the list of urls/commands
func Parse(r io.Reader) (Inputs, error) {
	s := &scanner{l: lex(r)}
	urls, err := parseConf(s)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}
	return urls, nil
}

type scanner struct {
	curItem  item
	prevItem *item
	l        *lexer
}

func (s *scanner) next() item {
	if s.prevItem == nil {
		s.curItem = <-s.l.items
		return s.curItem
	}
	i := *s.prevItem
	s.prevItem = nil
	return i
}

func parseConf(s *scanner) (Inputs, error) {
	var urls Inputs
	for i := s.next(); i.typ != itemEOF; i = s.next() {
		switch i.typ {
		case itemError:
			return nil, errors.New(i.val)
		case itemComment:
		case itemCmd, itemURL:
			urls = append(urls, i.val)
		default:
			return nil, fmt.Errorf("unexpected item (%s) in config file", i)
		}
	}
	return urls, nil
}
