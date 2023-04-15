package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.sr.ht/~ghost08/photon/lib/events"
	"git.sr.ht/~ghost08/photon/lib/inputs"
	"git.sr.ht/~ghost08/photon/lib/keybindings"
	"git.sr.ht/~ghost08/photon/lib/ls"
	"git.sr.ht/~ghost08/photon/lib/media"
	"git.sr.ht/~ghost08/photon/lib/states"
	"github.com/cjoudrey/gluahttp"
	lua "github.com/yuin/gopher-lua"
)

func (p *Photon) loadPlugins() error {
	plugins, err := findPlugins()
	if err != nil {
		return fmt.Errorf("finding plugins: %w", err)
	}
	if len(plugins) == 0 {
		return nil
	}
	p.initLuaState()
	for _, pluginPath := range plugins {
		if err := p.luaState.DoFile(pluginPath); err != nil {
			return fmt.Errorf("loading plugin: %s\n%w", pluginPath, err)
		}
	}
	return nil
}

func findPlugins() ([]string, error) {
	confDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	pluginsDir := filepath.Join(confDir, "photon", "plugins")
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		return nil, nil
	}
	des, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil, err
	}
	var plugins []string
	initPath := filepath.Join(confDir, "photon", "init.lua")
	if _, err := os.Stat(initPath); !os.IsNotExist(err) {
		plugins = []string{initPath}
	}
	for _, de := range des {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".lua") {
			continue
		}
		plugins = append(plugins, filepath.Join(pluginsDir, de.Name()))
	}
	return plugins, nil
}

var localStorage *ls.LocalStorage

func (p *Photon) initLuaState() {
	p.luaState = lua.NewState()
	media.Loader(p.luaState)
	p.cardsLoader(p.luaState)
	p.luaState.PreloadModule("photon", p.photonLoader)
	p.luaState.PreloadModule("http", gluahttp.NewHttpModule(p.httpClient).Loader)
	cache, _ := os.UserCacheDir()
	os.MkdirAll(filepath.Join(cache, "photon"), 0o755)
	localStorage = ls.New(filepath.Join(cache, "photon", "localStorage"))
	p.luaState.PreloadModule("localStorage", localStorage.Loader)
}

func (p *Photon) photonLoader(L *lua.LState) int {
	exports := map[string]lua.LGFunction{
		"state": p.state,
	}
	mod := L.SetFuncs(L.NewTable(), exports)

	// types and fields
	p.registerTypeSelectedCard(L)
	L.SetField(mod, "cards", newCards(&p.Cards, L))
	L.SetField(mod, "visibleCards", newCards(&p.VisibleCards, L))
	L.SetField(mod, "selectedCard", p.newSelectedCard(L))
	L.SetField(mod, "events", events.New(L))
	L.SetField(mod, "keybindings", keybindings.NewLValue(L, p.KeyBindings))
	L.SetField(mod, "feedInputs", inputs.New(L, p.feedInputs))

	// constants
	L.SetField(mod, "Normal", lua.LNumber(states.Normal))
	L.SetField(mod, "Article", lua.LNumber(states.Article))
	L.SetField(mod, "Search", lua.LNumber(states.Search))
	for n, c := range []string{
		"ColorBlack",
		"ColorMaroon",
		"ColorGreen",
		"ColorOlive",
		"ColorNavy",
		"ColorPurple",
		"ColorTeal",
		"ColorSilver",
		"ColorGray",
		"ColorRed",
		"ColorLime",
		"ColorYellow",
		"ColorBlue",
		"ColorFuchsia",
		"ColorAqua",
		"ColorWhite",
	} {
		L.SetField(mod, c, lua.LNumber(n))
	}

	L.Push(mod)

	return 1
}

func (p *Photon) state(L *lua.LState) int {
	L.Push(lua.LNumber(p.cb.State()))
	return 1
}

const luaSelectedCardTypeName = "photon.selectedCardType"

func (p *Photon) registerTypeSelectedCard(L *lua.LState) {
	selectedCardMethods := map[string]lua.LGFunction{
		"posX": func(L *lua.LState) int {
			scp := p.cb.SelectedCardPos()
			L.Push(lua.LNumber(scp.X))
			return 1
		},
		"posY": func(L *lua.LState) int {
			scp := p.cb.SelectedCardPos()
			L.Push(lua.LNumber(scp.Y))
			return 1
		},
		"card": func(L *lua.LState) int {
			L.Push(newCard(p.cb.SelectedCard(), L))
			return 1
		},
		"moveLeft": func(L *lua.LState) int {
			p.cb.Move().Left()
			return 0
		},
		"moveRight": func(L *lua.LState) int {
			p.cb.Move().Right()
			return 0
		},
		"moveUp": func(L *lua.LState) int {
			p.cb.Move().Up()
			return 0
		},
		"moveDown": func(L *lua.LState) int {
			p.cb.Move().Down()
			return 0
		},
	}
	newClass := L.SetFuncs(L.NewTable(), selectedCardMethods)
	mt := L.NewTypeMetatable(luaSelectedCardTypeName)
	L.SetField(mt, "__index", newClass)
}

func (p *Photon) newSelectedCard(L *lua.LState) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = p.cb.SelectedCard()
	L.SetMetatable(ud, L.GetTypeMetatable(luaSelectedCardTypeName))
	return ud
}
