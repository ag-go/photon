package keybindings

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Callback func() error

type KeyEvent struct {
	Key       rune
	Modifiers Modifiers
}

func (s KeyEvent) String() string {
	k := string(s.Key)
	switch s.Key {
	case '\u0008':
		k = "<backspace>"
	case '\t':
		k = "<tab>"
	case '\u00b1':
		k = "<esc>"
	case '\n':
		k = "<enter>"
	case 37:
		k = "<left>"
	case 38:
		k = "<up>"
	case 39:
		k = "<right>"
	case 40:
		k = "<down>"
	}
	return s.Modifiers.String() + k
}

type KeyEvents []KeyEvent

func (ss KeyEvents) String() string {
	var idents string
	for _, s := range ss {
		idents += s.String()
	}
	return idents
}

func parseState(s string) (string, KeyEvent, error) {
	ns, mod := modPrefix(s)
	if len(ns) == 0 {
		return "", KeyEvent{}, fmt.Errorf("not valid keybinding string: %s", s)
	}
	r, size := utf8.DecodeRuneInString(ns)
	r = unicode.ToLower(r)
	return ns[size:], KeyEvent{Key: r, Modifiers: mod}, nil
}

func modPrefix(s string) (string, Modifiers) {
	switch {
	case strings.HasPrefix(s, "<ctrl>"):
		return s[6:], ModCtrl
	case strings.HasPrefix(s, "<command>"):
		return s[9:], ModCommand
	case strings.HasPrefix(s, "<shift>"):
		return s[7:], ModShift
	case strings.HasPrefix(s, "<alt>"):
		return s[5:], ModAlt
	case strings.HasPrefix(s, "<super>"):
		return s[7:], ModSuper
	default:
		return s, 0
	}
}

func parseStates(s string) (KeyEvents, error) {
	var ss KeyEvents
	for {
		if len(s) == 0 {
			break
		}
		var err error
		var state KeyEvent
		s, state, err = parseState(s)
		if err != nil {
			return nil, err
		}
		ss = append(ss, state)
	}
	return ss, nil
}
