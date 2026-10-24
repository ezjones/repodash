package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezjones/repodash/internal/core"
)

// notesDialog is the box on the n key: the selected repo's notes, to read, add to or edit.
type notesDialog struct {
	name, title string
	text        string // the saved text the dialog shows
	edit        bool
	confirm     bool // asking "Save changes? y/n"
	scroll      int
	ta          textarea.Model
	x, y, w, h  int // where the box was last drawn, for mouse clicks
}

const dialogNarrow = 60

func (m *tuiModel) readNotes(name string) string {
	if m.d.ReadNotes != nil {
		t, _ := m.d.ReadNotes(name)
		return t
	}
	t, _ := core.ReadNotes(name)
	return t
}

func (m *tuiModel) saveNotes(name, text string) error {
	if m.d.SaveNotes != nil {
		return m.d.SaveNotes(name, text)
	}
	return core.SaveNotes(name, text)
}

// dialogSize is the box size for the current window.
func (m *tuiModel) dialogSize() (w, h int) {
	if m.width < dialogNarrow {
		return m.width, m.bodyH()
	}
	return min(m.width-6, 84), max(5, m.bodyH()-2)
}

func (m *tuiModel) openDialog() {
	r, ok := m.selected()
	if !ok {
		return
	}
	d := &notesDialog{name: r.Name, title: m.title(r)}
	d.text = m.readNotes(r.Name)
	m.dlg = d
	if strings.TrimSpace(d.text) == "" {
		d.text = ""
		m.startEdit()
	}
}

// startEdit switches the dialog to the editor, with the cursor at the end of the notes.
func (m *tuiModel) startEdit() {
	d := m.dlg
	w, h := m.dialogSize()
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.SetWidth(max(1, w-4))
	ta.SetHeight(max(1, h-3))
	ta.SetValue(d.text)
	ta.MoveToEnd()
	ta.Focus()
	d.ta, d.edit, d.confirm = ta, true, false
}

func (d *notesDialog) dirty() bool { return d.ta.Value() != d.text }

// saveDialog writes the editor's text and returns to view mode.
func (m *tuiModel) saveDialog() {
	d := m.dlg
	t := d.ta.Value()
	if err := m.saveNotes(d.name, t); err != nil {
		m.msg = err.Error()
		return
	}
	d.text, d.edit, d.confirm = t, false, false
	m.afterNotesSaved()
	m.msg = "notes saved"
}

// afterNotesSaved drops cached notes so the inspector and the card marker re-read them.
func (m *tuiModel) afterNotesSaved() { m.notes = nil }

func (m *tuiModel) dialogLines(w int) []string {
	return wrapWords(m.dlg.text, w)
}

func (m *tuiModel) dialogRows() int {
	_, h := m.dialogSize()
	return max(1, h-3)
}

func (m *tuiModel) dialogScroll(by int) {
	d := m.dlg
	d.scroll = min(max(d.scroll+by, 0), max(0, len(m.dialogLines(m.dialogW()))-m.dialogRows()))
}

func (m *tuiModel) dialogW() int {
	w, _ := m.dialogSize()
	return max(1, w-4)
}

// dialogKey handles every key while the dialog is open.
func (m *tuiModel) dialogKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := m.dlg
	s := k.String()
	if d.edit {
		if d.confirm {
			switch s {
			case "y":
				m.saveDialog()
			case "n":
				d.edit, d.confirm = false, false
			case "esc":
				d.confirm = false
			}
			return m, nil
		}
		switch s {
		case "ctrl+s":
			m.saveDialog()
		case "esc":
			switch {
			case d.dirty():
				d.confirm = true
			default:
				d.edit = false
			}
		default:
			var cmd tea.Cmd
			d.ta, cmd = d.ta.Update(k)
			return m, cmd
		}
		return m, nil
	}
	switch s {
	case "esc", "q":
		m.dlg = nil
	case "e", "enter":
		m.startEdit()
	case "o":
		name := d.name
		return m, m.editNotes(name)
	case "j", "down":
		m.dialogScroll(1)
	case "k", "up":
		m.dialogScroll(-1)
	case "pgdown", "space":
		m.dialogScroll(m.dialogRows())
	case "pgup":
		m.dialogScroll(-m.dialogRows())
	case "g", "home":
		d.scroll = 0
	case "G", "end":
		m.dialogScroll(1 << 20)
	}
	return m, nil
}

// dialogClick closes the dialog on a click outside it, in view mode only.
func (m *tuiModel) dialogClick(ms tea.Mouse) {
	d := m.dlg
	if ms.Button != tea.MouseLeft || d.edit {
		return
	}
	if ms.X < d.x || ms.X >= d.x+d.w || ms.Y < d.y || ms.Y >= d.y+d.h {
		m.dlg = nil
	}
}

// dialogBox draws the box and returns its lines.
func (m *tuiModel) dialogBox() []string {
	d := m.dlg
	w, h := m.dialogSize()
	inner := w - 2
	bs := fg(cInfo)
	title := " " + trunc(d.title+" notes", max(1, inner-2)) + " "
	top := bs.Render("╭") + bs.Bold(true).Render(title) + bs.Render(strings.Repeat("─", max(0, inner-lipgloss.Width(title)))+"╮")
	side := bs.Render("│")
	row := func(s string) string { return side + " " + padTo(s, inner-2) + " " + side }
	var body []string
	rows := h - 3
	var foot string
	switch {
	case d.edit && d.confirm:
		body = make([]string, rows)
		body[0] = fg(cYellow).Bold(true).Render("Save changes? y/n")
		foot = "y save · n discard · esc keep editing"
	case d.edit:
		body = strings.Split(d.ta.View(), "\n")
		foot = "ctrl+s save · esc cancel"
	case strings.TrimSpace(d.text) == "":
		body = []string{fg(cDim).Render("No notes yet. Press e to write some.")}
		foot = "e edit · o open in editor · esc close"
	default:
		ls := m.dialogLines(inner - 2)
		d.scroll = min(max(d.scroll, 0), max(0, len(ls)-rows))
		for _, l := range ls[d.scroll:min(len(ls), d.scroll+rows)] {
			body = append(body, fg(cText).Render(l))
		}
		foot = "e edit · o open in editor · esc close"
		if len(ls) > rows {
			foot = "j/k scroll · " + foot
		}
	}
	out := []string{top}
	for i := 0; i < rows; i++ {
		l := ""
		if i < len(body) {
			l = body[i]
		}
		out = append(out, row(l))
	}
	out = append(out, row(fg(cDim).Render(trunc(foot, inner-2))))
	out = append(out, bs.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

// overlayDialog draws the box centred over the body lines.
func (m *tuiModel) overlayDialog(body []string, bodyH int) []string {
	box := m.dialogBox()
	w, h := m.dialogSize()
	x, y := (m.width-w)/2, (bodyH-h)/2
	d := m.dlg
	d.x, d.y, d.w, d.h = x, y+1, w, h // +1 for the top bar
	for len(body) < bodyH {
		body = append(body, "")
	}
	for i, bl := range box {
		r := y + i
		if r < 0 || r >= len(body) {
			continue
		}
		l := body[r]
		left := ansi.Truncate(l, x, "")
		left += strings.Repeat(" ", max(0, x-lipgloss.Width(left)))
		right := ansi.TruncateLeft(l, x+w, "")
		body[r] = left + bl + right
	}
	return body
}
