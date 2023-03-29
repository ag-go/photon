package lib

import (
	"context"

	"git.sr.ht/~ghost08/photon/lib/media"
	lua "github.com/yuin/gopher-lua"
)

const (
	luaCardTypeName = "photon.card"
)

func (p *Photon) cardLoader(L *lua.LState) int {
	cardMethods := map[string]lua.LGFunction{
		"link":        cardItemLink,
		"image":       cardItemImage,
		"title":       cardItemTitle,
		"content":     cardItemContent,
		"description": cardItemDescription,
		"published":   cardItemPublished,
		"feed":        cardFeed,
		"getMedia":    getMedia,
		"runMedia": func(L *lua.LState) int {
			card := checkCard(L, 1)
			card.RunMedia()
			return 0
		},
		"openBrowser": func(L *lua.LState) int {
			card := checkCard(L, 1)
			_ = card.OpenBrowser()
			return 0
		},
		"openArticle": func(L *lua.LState) int {
			card := checkCard(L, 1)
			card.OpenArticle(context.Background())
			return 0
		},
		"foreground": func(L *lua.LState) int {
			card := checkCard(L, 1)
			color := L.CheckInt(2)
			card.Foreground = color
			p.cb.Redraw()
			return 0
		},
		"background": func(L *lua.LState) int {
			card := checkCard(L, 1)
			color := L.CheckInt(2)
			card.Background = color
			p.cb.Redraw()
			return 0
		},
	}
	mt := L.NewTypeMetatable(luaCardTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), cardMethods))
	return 0
}

func checkCard(L *lua.LState, index int) *Card {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.(*Card); ok {
		return v
	}
	L.ArgError(1, luaCardTypeName+" expected")
	return nil
}

func newCard(card *Card, L *lua.LState) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = card
	L.SetMetatable(ud, L.GetTypeMetatable(luaCardTypeName))
	return ud
}

func newCardFunc(card *Card) func(*lua.LState) lua.LValue {
	return func(L *lua.LState) lua.LValue {
		return newCard(card, L)
	}
}

func cardItemLink(L *lua.LState) int {
	card := checkCard(L, 1)
	if L.GetTop() == 2 {
		card.Item.Link = L.CheckString(2)
		return 0
	}
	L.Push(lua.LString(card.Item.Link))
	return 1
}

func cardItemImage(L *lua.LState) int {
	card := checkCard(L, 1)
	if L.GetTop() == 2 {
		card.Item.Image.URL = L.CheckString(2)
		return 0
	}
	L.Push(lua.LString(card.Item.Image.URL))
	return 1
}

func cardItemPublished(L *lua.LState) int {
	card := checkCard(L, 1)
	if L.GetTop() == 2 {
		card.Item.Published = L.CheckString(2)
		return 0
	}
	L.Push(lua.LString(card.Item.Published))
	return 1
}

func cardItemTitle(L *lua.LState) int {
	card := checkCard(L, 1)
	if L.GetTop() == 2 {
		card.Item.Title = L.CheckString(2)
		return 0
	}
	L.Push(lua.LString(card.Item.Title))
	return 1
}

func cardItemContent(L *lua.LState) int {
	card := checkCard(L, 1)
	if L.GetTop() == 2 {
		card.Item.Content = L.CheckString(2)
		return 0
	}
	L.Push(lua.LString(card.Item.Content))
	return 1
}

func cardItemDescription(L *lua.LState) int {
	card := checkCard(L, 1)
	if L.GetTop() == 2 {
		card.Item.Description = L.CheckString(2)
		return 0
	}
	L.Push(lua.LString(card.Item.Description))
	return 1
}

func getMedia(L *lua.LState) int {
	card := checkCard(L, 1)
	m, err := card.GetMedia()
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	ud := media.NewLuaMedia(m, L)
	L.Push(ud)
	L.Push(lua.LNil)
	return 2
}

func cardFeed(L *lua.LState) int {
	card := checkCard(L, 1)
	L.Push(newFeed(card.Feed, L))
	return 1
}
