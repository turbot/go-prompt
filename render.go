package prompt

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/c-bata/go-prompt/internal/debug"
)

// Render to render prompt information from state of Buffer.
type Render struct {
	out                ConsoleWriter
	prefix             string
	livePrefixCallback func() (prefix string, useLivePrefix bool)
	status             *string
	breakLineCallback  func(*Document)
	formatter          func(Document) ([]byte, error)
	title              string
	row                uint16
	col                uint16

	previousCursor     int
	lastStatusRendered *string

	// colors,
	statusTextColor              Color
	statusBGColor                Color
	prefixTextColor              Color
	prefixBGColor                Color
	inputTextColor               Color
	inputBGColor                 Color
	previewSuggestionTextColor   Color
	previewSuggestionBGColor     Color
	suggestionTextColor          Color
	suggestionBGColor            Color
	selectedSuggestionTextColor  Color
	selectedSuggestionBGColor    Color
	descriptionTextColor         Color
	descriptionBGColor           Color
	selectedDescriptionTextColor Color
	selectedDescriptionBGColor   Color
	scrollbarThumbColor          Color
	scrollbarBGColor             Color

	renderLock *sync.Mutex
}

// Setup to initialize console output.
func (r *Render) Setup() {
	if r.title != "" {
		r.out.SetTitle(r.title)
		debug.AssertNoError(r.out.Flush())
	}
}

// getCurrentPrefix to get current prefix.
// If live-prefix is enabled, return live-prefix.
func (r *Render) getCurrentPrefix() string {
	if prefix, ok := r.livePrefixCallback(); ok {
		return prefix
	}
	return r.prefix
}

func (r *Render) renderStatus() {
	r.out.SetColor(r.statusTextColor, r.statusBGColor, false)
	if r.status != nil {
		r.out.WriteStr(fmt.Sprintf("%s\n", *r.status))
	}
	r.out.SetColor(DefaultColor, DefaultColor, false)
	r.lastStatusRendered = r.status
}

func (r *Render) renderPrefix() {
	r.out.SetColor(r.prefixTextColor, r.prefixBGColor, false)
	r.out.WriteStr(r.getCurrentPrefix())
	r.out.SetColor(DefaultColor, DefaultColor, false)
}

// TearDown to clear title and erasing.
func (r *Render) TearDown() {
	r.out.ClearTitle()
	r.out.EraseDown()
	debug.AssertNoError(r.out.Flush())
}

func (r *Render) prepareArea(lines int) {
	for i := 0; i < lines; i++ {
		r.out.ScrollDown()
	}
	for i := 0; i < lines; i++ {
		r.out.ScrollUp()
	}
}

// UpdateWinSize called when window size is changed.
func (r *Render) UpdateWinSize(ws *WinSize) {
	r.row = ws.Row
	r.col = ws.Col
}

func (r *Render) renderWindowTooSmall() {
	r.out.CursorGoTo(0, 0)
	r.out.EraseScreen()
	r.out.SetColor(DarkRed, White, false)
	r.out.WriteStr("Your console window is too small...")
}

func (r *Render) renderCompletion(buf *Buffer, completions *CompletionManager, availableRows int) {
	defer debug.Un(debug.Trace("renderCompletion"))
	suggestions := completions.GetSuggestions()
	if len(completions.GetSuggestions()) == 0 {
		return
	}
	prefix := r.getCurrentPrefix()
	formatted, width := formatSuggestions(
		suggestions,
		int(r.col)-utf8.RuneCountInString(prefix)-1, // -1 means a width of scrollbar
	)
	// +1 means a width of scrollbar.
	width++

	requiredRows := len(formatted)

	if availableRows < 3 && requiredRows > 3 {
		debug.Log("Not enough height to render suggestions")
		return
	}

	viewportRows := requiredRows
	if viewportRows > int(completions.max) {
		debug.Log("windowHeight > int(completions.max)")
		viewportRows = int(completions.max)
	}
	if viewportRows > availableRows {
		debug.Log("windowHeight > availableRows")
		viewportRows = availableRows
	}

	completions.viewportRows = viewportRows

	formatted = formatted[completions.verticalScroll : completions.verticalScroll+viewportRows]
	r.prepareArea(viewportRows)

	cursor := utf8.RuneCountInString(prefix) + utf8.RuneCountInString(buf.Document().TextBeforeCursor())
	x, _ := r.toPos(cursor, buf.Text())
	if x+width >= int(r.col) {
		cursor = r.backward(cursor, x+width-int(r.col), buf.Text())
	}

	contentHeight := len(completions.tmp)

	fractionVisible := float64(viewportRows) / float64(contentHeight)
	fractionAbove := float64(completions.verticalScroll) / float64(contentHeight)

	scrollbarHeight := int(clamp(float64(viewportRows), 1, float64(viewportRows)*fractionVisible))
	scrollbarTop := int(float64(viewportRows) * fractionAbove)

	isScrollThumb := func(row int) bool {
		return scrollbarTop <= row && row <= scrollbarTop+scrollbarHeight
	}

	selected := completions.selected - completions.verticalScroll
	r.out.SetColor(White, Cyan, false)
	for i := 0; i < viewportRows; i++ {
		r.out.CursorDown(1)
		if i == selected {
			r.out.SetColor(r.selectedSuggestionTextColor, r.selectedSuggestionBGColor, true)
		} else {
			r.out.SetColor(r.suggestionTextColor, r.suggestionBGColor, false)
		}
		r.out.WriteStr(formatted[i].Text)

		if i == selected {
			r.out.SetColor(r.selectedDescriptionTextColor, r.selectedDescriptionBGColor, false)
		} else {
			r.out.SetColor(r.descriptionTextColor, r.descriptionBGColor, false)
		}
		r.out.WriteStr(formatted[i].Description)

		if isScrollThumb(i) {
			r.out.SetColor(DefaultColor, r.scrollbarThumbColor, false)
		} else {
			r.out.SetColor(DefaultColor, r.scrollbarBGColor, false)
		}
		r.out.WriteStr(" ")
		r.out.SetColor(DefaultColor, DefaultColor, false)

		r.lineWrap(cursor + width)
		r.backward(cursor+width, width, "")
	}

	if x+width >= int(r.col) {
		r.out.CursorForward(x + width - int(r.col))
	}

	r.out.CursorUp(viewportRows)
	r.out.SetColor(DefaultColor, DefaultColor, false)
}

// ClearScreen :: Clears the screen and moves the cursor to home
func (r *Render) ClearScreen() {
	r.out.EraseScreen()
	r.out.CursorGoTo(0, 0)
}

// Render renders to the console.
func (r *Render) Render(buffer *Buffer, previousText string, completion *CompletionManager) {
	defer debug.Un(debug.Trace("Render"))

	// to make sure that renders triggers from different sources
	// (kbd event, prompt.Render, prompt.SetStatus)
	// do not race with each other
	r.renderLock.Lock()
	defer r.renderLock.Unlock()

	// In situations where a pseudo tty is allocated (e.g. within a docker container),
	// window size via TIOCGWINSZ is not immediately available and will result in 0,0 dimensions.
	if r.col == 0 {
		return
	}
	defer func() { debug.AssertNoError(r.out.Flush()) }()

	line := buffer.Text()
	prefix := r.getCurrentPrefix()
	cursor := utf8.RuneCountInString(prefix) + utf8.RuneCountInString(line)

	r.move(r.previousCursor, 0, previousText)

	// prepare area
	r.toPos(cursor, line)
	if completionMargin > int(r.col) {
		r.renderWindowTooSmall()
		return
	}

	// Rendering
	r.out.HideCursor()
	defer r.out.ShowCursor()

	r.out.EraseLine()
	r.out.EraseDown()

	if r.lastStatusRendered != nil {
		r.out.CursorUp(1)
	}
	r.renderStatus()
	r.renderPrefix()

	var formatted []byte
	if r.formatter != nil {
		if formattedBytes, err := r.formatter(*buffer.Document()); err == nil {
			// the formatter gives back a fully formatted text
			// with all control characters
			formatted = formattedBytes
		}
	}

	if len(formatted) == 0 {
		r.out.SetColor(r.inputTextColor, r.inputBGColor, false)
		r.out.WriteStr(line)
		r.out.SetColor(DefaultColor, DefaultColor, false)
	} else {
		// the formatted text contains all necessary control characters
		r.out.WriteRaw(formatted)
	}
	r.lineWrap(cursor)

	r.out.EraseDown()

	cursor = r.backward(cursor, utf8.RuneCountInString(line)-buffer.DisplayCursorPosition(), buffer.Text())

	r.renderCompletion(buffer, completion, int(r.row)-4)
	if suggest, ok := completion.GetSelectedSuggestion(); ok {
		cursor = r.backward(cursor, utf8.RuneCountInString(buffer.Document().GetWordBeforeCursorUntilSeparator(completion.wordSeparator)), buffer.Text())

		r.out.SetColor(r.previewSuggestionTextColor, r.previewSuggestionBGColor, false)
		r.out.WriteStr(suggest.Text)
		r.out.SetColor(DefaultColor, DefaultColor, false)
		cursor += utf8.RuneCountInString(suggest.Text)

		rest := buffer.Document().TextAfterCursor()
		r.out.WriteStr(rest)
		cursor += utf8.RuneCountInString(rest)
		r.lineWrap(cursor)

		cursor = r.backward(cursor, utf8.RuneCountInString(rest), rest)
	}
	r.previousCursor = cursor
}

// BreakLine to break line.
func (r *Render) BreakLine(buffer *Buffer) {
	defer debug.Un(debug.Trace("BreakLine", buffer.Text()))
	// Erasing and Render
	cursor := utf8.RuneCountInString(buffer.Document().TextBeforeCursor()) + utf8.RuneCountInString(r.getCurrentPrefix())
	r.clear(cursor, buffer.Document().TextBeforeCursor())
	r.renderPrefix()

	var formatted []byte
	if r.formatter != nil {
		if formattedBytes, err := r.formatter(*buffer.Document()); err == nil {
			// the formatter gives back a fully formatted text
			// with all control characters
			formatted = formattedBytes
		}
	}

	if len(formatted) == 0 {
		r.out.SetColor(r.inputTextColor, r.inputBGColor, false)
		r.out.WriteStr(buffer.Document().Text + "\n")
		r.out.SetColor(DefaultColor, DefaultColor, false)
	} else {
		// the formatted text contains all necessary control characters
		r.out.WriteRaw(formatted)
		r.out.WriteRawStr("\n")
	}

	debug.AssertNoError(r.out.Flush())
	if r.breakLineCallback != nil {
		r.breakLineCallback(buffer.Document())
	}

	r.previousCursor = 0
}

// clear erases the screen from a beginning of input
// even if there is line break which means input length exceeds a window's width.
func (r *Render) clear(cursor int, text string) {
	defer debug.Un(debug.Trace("clear", cursor, text))
	r.move(cursor, 0, text)
	r.out.EraseLine()
	r.out.EraseDown()
}

// backward moves cursor to backward from a current cursor position
// regardless there is a line break.
func (r *Render) backward(from, n int, text string) int {
	defer debug.Un(debug.Trace("backward", from, n))
	return r.move(from, from-n, text)
}

// move moves cursor to specified position from the beginning of input
// even if there is a line break.
func (r *Render) move(from, to int, text string) int {
	defer debug.Un(debug.Trace("move", from, to))
	fromX, fromY := r.toPos(from, text)
	toX, toY := r.toPos(to, text)

	debug.Log(fmt.Sprintf("From: {%v,%v}\n", fromX, fromY))
	debug.Log(fmt.Sprintf("To  : {%v,%v}\n", toX, toY))

	r.out.CursorUp(fromY - toY)
	r.out.CursorBackward(fromX - toX)
	return to
}

// toPos returns the relative position from the beginning of the string.
func (r *Render) toPos(cursor int, text string) (x, y int) {
	defer debug.Un(debug.Trace("toPos", cursor, text))

	defer func() {
		debug.Log(fmt.Sprintf("Returning: {%v,%v}\n", x, y))
	}()

	// var start, end, lineCount int
	cols := int(r.col)

	if strings.Count(text, "\n") == 0 {
		x = cursor % cols
		y = cursor / cols
		return
	}

	var start, end, lineCount int
	text = fmt.Sprintf("%s%s", r.getCurrentPrefix(), text)
	for _, line := range strings.Split(text, "\n") {
		line = fmt.Sprintf("%s\n", line)
		length := utf8.RuneCountInString(line)
		end = start + length - 1
		if end >= cursor {
			x := (cursor - start) % cols
			y := lineCount + (cursor-start)/cols
			return x, y
		}
		// if length 0 to cols, add 1
		// if length cols+1 to (2*cols)-1, add 2 - etc.
		lineCount += ((length - 1) / cols) + 1
		start = end + 1
	}
	return
}

func (r *Render) lineWrap(cursor int) {
	if runtime.GOOS != "windows" && cursor > 0 && cursor%int(r.col) == 0 {
		r.out.WriteRaw([]byte{'\n'})
	}
}

func clamp(high, low, x float64) float64 {
	switch {
	case high < x:
		return high
	case x < low:
		return low
	default:
		return x
	}
}
