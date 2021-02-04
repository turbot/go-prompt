package prompt

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/c-bata/go-prompt/internal/debug"
	runewidth "github.com/mattn/go-runewidth"
)

// Render to render prompt information from state of Buffer.
type Render struct {
	out                ConsoleWriter
	prefix             string
	livePrefixCallback func() (prefix string, useLivePrefix bool)
	breakLineCallback  func(*Document)
	title              string
	row                uint16
	col                uint16

	previousCursor int

	// colors,
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

func (r *Render) renderCompletion(buf *Buffer, completions *CompletionManager) {
	suggestions := completions.GetSuggestions()
	if len(completions.GetSuggestions()) == 0 {
		return
	}
	prefix := r.getCurrentPrefix()
	formatted, width := formatSuggestions(
		suggestions,
		int(r.col)-runewidth.StringWidth(prefix)-1, // -1 means a width of scrollbar
	)
	// +1 means a width of scrollbar.
	width++

	windowHeight := len(formatted)
	if windowHeight > int(completions.max) {
		windowHeight = int(completions.max)
	}
	formatted = formatted[completions.verticalScroll : completions.verticalScroll+windowHeight]
	r.prepareArea(windowHeight)

	cursor := runewidth.StringWidth(prefix) + runewidth.StringWidth(buf.Document().TextBeforeCursor())
	x, _ := r.toPos(cursor, buf.Document().TextBeforeCursor())
	if x+width >= int(r.col) {
		cursor = r.backward(cursor, x+width-int(r.col))
	}

	contentHeight := len(completions.tmp)

	fractionVisible := float64(windowHeight) / float64(contentHeight)
	fractionAbove := float64(completions.verticalScroll) / float64(contentHeight)

	scrollbarHeight := int(clamp(float64(windowHeight), 1, float64(windowHeight)*fractionVisible))
	scrollbarTop := int(float64(windowHeight) * fractionAbove)

	isScrollThumb := func(row int) bool {
		return scrollbarTop <= row && row <= scrollbarTop+scrollbarHeight
	}

	selected := completions.selected - completions.verticalScroll
	r.out.SetColor(White, Cyan, false)
	for i := 0; i < windowHeight; i++ {
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
		r.backward(cursor+width, width)
	}

	if x+width >= int(r.col) {
		r.out.CursorForward(x + width - int(r.col))
	}

	r.out.CursorUp(windowHeight)
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
	// In situations where a pseudo tty is allocated (e.g. within a docker container),
	// window size via TIOCGWINSZ is not immediately available and will result in 0,0 dimensions.
	if r.col == 0 {
		return
	}
	defer func() { debug.AssertNoError(r.out.Flush()) }()

	// previousCursorIndexInBuffer := r.previousCursor

	line := strings.TrimSpace(buffer.Text())
	traceBackLines := strings.Count(previousText, "\n")
	if len(line) == 0 {
		// if the new buffer is empty, then we shouldn't traceback any
		traceBackLines = 0
	}
	debug.Log(fmt.Sprintln("TraceBackLines:", traceBackLines))
	debug.Log(fmt.Sprintln("r.col:", r.col))
	debug.Log(fmt.Sprintln("r.previousCursor:", r.previousCursor))

	debug.Log(fmt.Sprintf("moving: %d", (traceBackLines)*int(r.col)+r.previousCursor))
	r.moveForRender(r.previousCursor, 0, previousText)

	prefix := r.getCurrentPrefix()
	cursor := runewidth.StringWidth(prefix) + runewidth.StringWidth(line)
	debug.Log(fmt.Sprintf("cursor: %d", cursor))

	// prepare area
	debug.Log(fmt.Sprintf("Calculating toPos from: %d", (traceBackLines+int(r.col))+cursor))
	_, y := r.toPos(cursor, line)
	debug.Log(fmt.Sprintf("y: %d", y))

	h := y + 1 + int(completion.max)
	if h > int(r.row) || completionMargin > int(r.col) {
		r.renderWindowTooSmall()
		return
	}

	// Rendering
	r.out.HideCursor()

	r.out.EraseLine()
	r.out.EraseDown()

	r.renderPrefix()

	if buffer.NewLineCount() > 0 {
		r.renderMultiline(buffer)
	} else {
		r.out.WriteStr(line)
		defer r.out.ShowCursor()
	}

	r.lineWrap(cursor)
	r.out.SetColor(DefaultColor, DefaultColor, false)

	cursor = r.backward(cursor, runewidth.StringWidth(line)-buffer.DisplayCursorPosition())

	r.renderCompletion(buffer, completion)
	if suggest, ok := completion.GetSelectedSuggestion(); ok {
		cursor = r.backward(cursor, runewidth.StringWidth(buffer.Document().GetWordBeforeCursorUntilSeparator(completion.wordSeparator)))

		r.out.SetColor(r.previewSuggestionTextColor, r.previewSuggestionBGColor, false)
		r.out.WriteStr(suggest.Text)
		r.out.SetColor(DefaultColor, DefaultColor, false)
		cursor += runewidth.StringWidth(suggest.Text)

		rest := buffer.Document().TextAfterCursor()
		r.out.WriteStr(rest)
		cursor += runewidth.StringWidth(rest)
		r.lineWrap(cursor)

		cursor = r.backward(cursor, runewidth.StringWidth(rest))
	}
	r.previousCursor = cursor
}

func (r *Render) renderMultiline(buffer *Buffer) {
	// defer debug.Un(debug.Trace("renderMultiline"))
	before := buffer.Document().TextBeforeCursor()
	cursor := ""
	after := ""

	if runewidth.StringWidth(buffer.Document().TextAfterCursor()) == 0 {
		cursor = " "
		after = ""
	} else {
		cursor = string(buffer.Text()[buffer.Document().cursorPosition])
		if cursor == "\n" {
			cursor = " \n"
		}
		after = buffer.Document().TextAfterCursor()[1:]
	}

	//	debug.Log(fmt.Sprintln("before:", before))
	//	debug.Log(fmt.Sprintln("cursor:", cursor))
	//	debug.Log(fmt.Sprintln("after :", after))

	r.out.SetColor(r.inputTextColor, r.inputBGColor, false)
	r.out.WriteStr(before)

	r.out.SetDisplayAttributes(r.inputTextColor, r.inputBGColor, DisplayReverse)
	r.out.WriteStr(cursor)

	r.out.SetColor(r.inputTextColor, r.inputBGColor, false)
	r.out.WriteStr(after)
}

// BreakLine to break line.
func (r *Render) BreakLine(buffer *Buffer) {
	defer debug.Un(debug.Trace("BreakLine", buffer.Text()))
	// Erasing and Render
	cursor := runewidth.StringWidth(buffer.Document().TextBeforeCursor()) + runewidth.StringWidth(r.getCurrentPrefix())
	r.clear(cursor, buffer.Document().TextBeforeCursor())
	r.renderPrefix()
	r.out.SetColor(r.inputTextColor, r.inputBGColor, false)
	r.out.WriteStr(buffer.Document().Text + "\n")
	r.out.SetColor(DefaultColor, DefaultColor, false)
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
	r.moveForRender(cursor, 0, text)
	r.out.EraseLine()
	r.out.EraseDown()
}

// backward moves cursor to backward from a current cursor position
// regardless there is a line break.
func (r *Render) backward(from, n int) int {
	// defer debug.Un(debug.Trace("backward", from, n))
	return r.move(from, from-n)
}

// move moves cursor to specified position from the beginning of input
// even if there is a line break.
func (r *Render) move(from, to int) int {
	defer debug.Un(debug.Trace("move", from, to))
	fromX, fromY := r.toPos(from, "")
	toX, toY := r.toPos(to, "")

	debug.Log(fmt.Sprintf("From: {%v,%v}\n", fromX, fromY))
	debug.Log(fmt.Sprintf("To  : {%v,%v}\n", toX, toY))

	r.out.CursorUp(fromY - toY)
	r.out.CursorBackward(fromX - toX)
	return to
}

func (r *Render) moveForRender(from, to int, text string) int {
	defer debug.Un(debug.Trace("moveForRender", from, to))
	text = fmt.Sprintf("%s%s", r.getCurrentPrefix(), text)

	lineCount := strings.Count(text, "\n")
	col := int(r.col)

	fromX, fromY := r.toPos((col*lineCount)+from, text)
	toX, toY := r.toPos(to, text)

	debug.Log(fmt.Sprintf("From: {%v,%v}\n", fromX, fromY))
	debug.Log(fmt.Sprintf("To  : {%v,%v}\n", toX, toY))

	r.out.CursorUp(fromY - toY)
	r.out.CursorBackward(fromX - toX)
	return to
}

// toPos returns the relative position from the beginning of the string.
func (r *Render) toPos(cursor int, text string) (x, y int) {
	defer debug.Un(debug.Trace("toPos", cursor))

	// var start, end, lineCount int
	cols := int(r.col)

	type Block struct {
		start int
		end   int
	}

	lineBlocks := []Block{}

	// calculate the locations of the start and end of each line
	for idx, line := range strings.Split(text, "\n") {

		// put the newline back in
		line := fmt.Sprintf("%s\n", line)

		newBlocks := []Block{}
		thisBlock := Block{}

		if idx == 0 {
			thisBlock.start = 0
			thisBlock.end = (len(line) - 1)
		} else {
			thisBlock.start = lineBlocks[idx-1].end + 1
			thisBlock.end = thisBlock.start + (len(line) - 1)
		}

		// normalize by max columns
		for {
			lengthOfThisBlock := thisBlock.end - thisBlock.start
			if lengthOfThisBlock > cols {
				newBlock := Block{
					start: thisBlock.start,
					end:   thisBlock.start + cols - 1,
				}
				// put the generated block in
				newBlocks = append(newBlocks, newBlock)

				thisBlock = Block{
					start: thisBlock.start + cols,
					end:   thisBlock.end,
				}
			}
			break
		}

		// put in the remaining block
		newBlocks = append(newBlocks, thisBlock)

		lineBlocks = append(lineBlocks, newBlocks...)

	}

	for idx, block := range lineBlocks {
		if cursor > block.start && cursor < block.end {
			y = idx
			x = cursor - block.start
		}
	}

	// lines := strings.Split(text, "\n")
	// for i, line := range lines {
	// 	length := runewidth.StringWidth(line)
	// 	start = end
	// 	end += length
	// 	lineCount += (length / cols) + 1
	// 	if end > cursor {
	// 		y = i
	// 		break
	// 	}
	// 	x = (cursor - start) % cols
	// }
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
