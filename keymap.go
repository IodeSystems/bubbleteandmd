package bubbleteandmd

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

// KeyMap binds each Action to keys, named as tea.KeyPressMsg.String names
// them ("tab", "shift+tab", "enter", "space", "left"). An action with no
// keys is off. The scroll keys act only in a Doc with a height; at height
// 0 HandleKey returns false for them and the host scrolls.
type KeyMap map[Action][]string

// DefaultKeyMap: tab and shift+tab move between stops, enter and space
// toggle, ←/→ change the tab or the tree folder, esc leaves; with a height,
// ↑/↓ scroll a line, pgup/pgdown a page, home/end to the top/bottom.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		ActionNext:       {"tab"},
		ActionPrev:       {"shift+tab"},
		ActionToggle:     {"enter", "space"},
		ActionLeft:       {"left"},
		ActionRight:      {"right"},
		ActionLeave:      {"esc"},
		ActionScrollUp:   {"up"},
		ActionScrollDown: {"down"},
		ActionPageUp:     {"pgup"},
		ActionPageDown:   {"pgdown"},
		ActionTop:        {"home"},
		ActionBottom:     {"end"},
	}
}

// Action returns the action key is bound to; actions are looked up in the
// order of Actions, so a key bound twice is the earlier action's.
func (k KeyMap) Action(key string) (Action, bool) {
	for _, a := range Actions {
		if slices.Contains(k[a], key) {
			return a, true
		}
	}
	return "", false
}

// Label is how a hint names an action's key: its first, or "" if it has
// none.
func (k KeyMap) Label(a Action) string {
	if ks := k[a]; len(ks) > 0 {
		return ks[0]
	}
	return ""
}

// HandleKey does the action msg is bound to in km and reports whether the
// key was consumed (Do). False: the host handles the key.
func (d *Doc) HandleKey(msg tea.KeyPressMsg, km KeyMap) bool {
	a, ok := km.Action(msg.String())
	return ok && d.Do(a)
}
