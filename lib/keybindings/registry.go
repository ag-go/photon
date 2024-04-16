package keybindings

import (
	"log"
	"strings"
	"unicode"

	"git.sr.ht/~ghost08/photon/lib/states"
)

type Registry struct {
	currentLayout states.Func
	reg           map[states.Enum]map[string]Callback
	currentState  KeyEvents
	repeat        int
}

func NewRegistry(cl states.Func) *Registry {
	return &Registry{
		reg:           make(map[states.Enum]map[string]Callback),
		currentLayout: cl,
	}
}

func (kbr *Registry) Add(layout states.Enum, keyString string, callback Callback) {
	if _, ok := kbr.reg[layout]; !ok {
		kbr.reg[layout] = make(map[string]Callback)
	}
	ss, err := parseStates(keyString)
	if err != nil {
		log.Printf("ERROR: parsing keybinding string (%s): %s", keyString, err)
		return
	}
	kbr.reg[layout][ss.String()] = callback
}

func (kbr *Registry) Run(e KeyEvent) {
	cl := kbr.currentLayout()
	reg, ok := kbr.reg[cl]
	if !ok {
		return
	}
	if e.Modifiers == 0 && unicode.IsDigit(e.Key) && len(kbr.currentState) == 0 {
		kbr.repeat = kbr.repeat*10 + (int(e.Key) - '0')
	}
	kbr.currentState = append(kbr.currentState, e)
	ident := kbr.currentState.String()
	var hasPrefix bool
	var callback Callback
	for k, c := range reg {
		if !strings.HasPrefix(k, ident) {
			continue
		}
		hasPrefix = true
		if len(ident) == len(k) {
			callback = c
			break
		}
	}
	if !hasPrefix {
		kbr.currentState = nil
		return
	}
	if callback == nil {
		return
	}
	kbr.currentState = nil
	if kbr.repeat == 0 {
		if err := callback(); err != nil {
			log.Println("ERROR:", err)
		}
		return
	}
	for range kbr.repeat {
		if err := callback(); err != nil {
			log.Println("ERROR:", err)
			break
		}
	}
	kbr.repeat = 0
}
