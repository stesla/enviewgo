package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input    string
		expected []text
	}{
		{"", []text{}},
		{"plain", []text{text{text: "plain"}}},

		// low colors
		{"some\x1b[36;44mcolor", []text{
			{text: "some"},
			{text: "color", bg: "#000080", fg: "#008080"}}},

		// high colors
		{"some\x1b[96;104mcolor", []text{
			{text: "some"},
			{text: "color", bg: "#0000ff", fg: "#00ffff"}}},

		// 256-color low
		{"\x1b[38;5;1;48;5;2mword", []text{
			{text: "word", bg: "#008000", fg: "#800000"}}},

		// 256-color high
		{"\x1b[38;5;8;48;5;9mword", []text{
			{text: "word", bg: "#ff0000", fg: "#808080"}}},

		// color-cube black
		{"\x1b[38;5;16mword", []text{{text: "word", fg: "#000000"}}},

		// color-cube color
		{"\x1b[38;5;42mword", []text{{text: "word", fg: "#00d787"}}},

		// grayscale color
		{"\x1b[38;5;243mword", []text{{text: "word", fg: "#767676"}}},

		// reset
		{"foo\x1b[31mbar\x1b[0mbaz", []text{
			{text: "foo"},
			{text: "bar", fg: "#800000"},
			{text: "baz"}}},

		// bold
		{"\x1b[1mfoo", []text{{text: "foo", bold: true}}},

		// multiple sequences
		{"\x1b[1m\x1b[33mfoo", []text{
			{text: "foo", bold: true, fg: "#808000"}}},
	}
	for _, test := range tests {
		actual, err := parse(test.input)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, actual)
	}
}
