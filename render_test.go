// +build !windows

package prompt

import (
	"reflect"
	"syscall"
	"testing"
)

func TestFormatCompletion(t *testing.T) {
	scenarioTable := []struct {
		scenario      string
		completions   []Suggest
		prefix        string
		suffix        string
		expected      []Suggest
		maxWidth      int
		expectedWidth int
	}{
		{
			scenario: "",
			completions: []Suggest{
				{Text: "select"},
				{Text: "from"},
				{Text: "insert"},
				{Text: "where"},
			},
			prefix: " ",
			suffix: " ",
			expected: []Suggest{
				{Text: " select "},
				{Text: " from   "},
				{Text: " insert "},
				{Text: " where  "},
			},
			maxWidth:      20,
			expectedWidth: 8,
		},
		{
			scenario: "",
			completions: []Suggest{
				{Text: "select", Description: "select description"},
				{Text: "from", Description: "from description"},
				{Text: "insert", Description: "insert description"},
				{Text: "where", Description: "where description"},
			},
			prefix: " ",
			suffix: " ",
			expected: []Suggest{
				{Text: " select ", Description: " select description "},
				{Text: " from   ", Description: " from description   "},
				{Text: " insert ", Description: " insert description "},
				{Text: " where  ", Description: " where description  "},
			},
			maxWidth:      40,
			expectedWidth: 28,
		},
	}

	for _, s := range scenarioTable {
		ac, width := formatSuggestions(s.completions, s.maxWidth)
		if !reflect.DeepEqual(ac, s.expected) {
			t.Errorf("Should be %#v, but got %#v", s.expected, ac)
		}
		if width != s.expectedWidth {
			t.Errorf("Should be %#v, but got %#v", s.expectedWidth, width)
		}
	}
}

func TestToPos(t *testing.T) {
	type Coordinate struct {
		x int
		y int
	}
	scenarioTable := []struct {
		scenario string
		cols     int
		text     string
		cursor   int
		expected Coordinate
	}{
		{
			scenario: "Single line",
			cols:     20,
			text:     "0123456",
			cursor:   3,
			expected: Coordinate{3, 0},
		},
		{
			scenario: "Multi line",
			cols:     20,
			text:     "0123456\n0\n01234\n0123456789",
			cursor:   13,
			expected: Coordinate{3, 2},
		},
		{
			scenario: "Multi line with overflow in one line",
			cols:     20,
			text:     "0123456\n012345678901234567890123\n01234\n0123456789",
			cursor:   13,
			expected: Coordinate{5, 1},
		},
		{
			scenario: "Multi line with overflow in two line",
			cols:     20,
			text:     "0123456\n012345678901234567890123\n01234\n012345678901234567890123",
			cursor:   13,
			expected: Coordinate{5, 1},
		},
		{
			scenario: "Multi line with overflow in two line",
			cols:     20,
			text:     "0123456\n012345678901234567890123\n01234\n012345678901234567890123",
			cursor:   0,
			expected: Coordinate{0, 0},
		},
	}

	r := &Render{
		prefix: "> ",
		out: &PosixWriter{
			fd: syscall.Stdin, // "write" to stdin just so we don't mess with the output of the tests
		},
		livePrefixCallback:           func() (string, bool) { return "", false },
		prefixTextColor:              Blue,
		prefixBGColor:                DefaultColor,
		inputTextColor:               DefaultColor,
		inputBGColor:                 DefaultColor,
		previewSuggestionTextColor:   Green,
		previewSuggestionBGColor:     DefaultColor,
		suggestionTextColor:          White,
		suggestionBGColor:            Cyan,
		selectedSuggestionTextColor:  Black,
		selectedSuggestionBGColor:    Turquoise,
		descriptionTextColor:         Black,
		descriptionBGColor:           Turquoise,
		selectedDescriptionTextColor: White,
		selectedDescriptionBGColor:   Cyan,
		scrollbarThumbColor:          DarkGray,
		scrollbarBGColor:             Cyan,
		col:                          1,
	}

	for _, s := range scenarioTable {
		r.col = uint16(s.cols)
		x, y := r.toPos(s.cursor, s.text)

		coord := Coordinate{x, y}

		if !reflect.DeepEqual(coord, s.expected) {
			t.Errorf("Should be %#v, but got %#v", s.expected, coord)
		}
	}
}

func TestBreakLineCallback(t *testing.T) {
	var i int
	r := &Render{
		prefix: "> ",
		out: &PosixWriter{
			fd: syscall.Stdin, // "write" to stdin just so we don't mess with the output of the tests
		},
		livePrefixCallback:           func() (string, bool) { return "", false },
		prefixTextColor:              Blue,
		prefixBGColor:                DefaultColor,
		inputTextColor:               DefaultColor,
		inputBGColor:                 DefaultColor,
		previewSuggestionTextColor:   Green,
		previewSuggestionBGColor:     DefaultColor,
		suggestionTextColor:          White,
		suggestionBGColor:            Cyan,
		selectedSuggestionTextColor:  Black,
		selectedSuggestionBGColor:    Turquoise,
		descriptionTextColor:         Black,
		descriptionBGColor:           Turquoise,
		selectedDescriptionTextColor: White,
		selectedDescriptionBGColor:   Cyan,
		scrollbarThumbColor:          DarkGray,
		scrollbarBGColor:             Cyan,
		col:                          1,
	}
	b := NewBuffer()
	r.BreakLine(b)

	if i != 0 {
		t.Errorf("i should initially be 0, before applying a break line callback")
	}

	r.breakLineCallback = func(doc *Document) {
		i++
	}
	r.BreakLine(b)
	r.BreakLine(b)
	r.BreakLine(b)

	if i != 3 {
		t.Errorf("BreakLine callback not called, i should be 3")
	}
}
