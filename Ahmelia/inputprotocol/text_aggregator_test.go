package inputprotocol

import (
	"reflect"
	"testing"
	"time"
)

func TestCoalesceTextLinesPreservesMultilineTurn(t *testing.T) {
	lines := make(chan string, 4)
	lines <- "Quiero que implementes una utilidad."
	lines <- ""
	lines <- "Objetivo:"
	lines <- "Crea una función Go llamada IsEven."
	close(lines)

	var got []string
	CoalesceTextLines(lines, time.Hour, func(text string) { got = append(got, text) })

	want := []string{"Quiero que implementes una utilidad.\n\nObjetivo:\nCrea una función Go llamada IsEven."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCoalesceTextLinesFlushesAfterIdleBoundary(t *testing.T) {
	lines := make(chan string, 2)
	go func() {
		lines <- "primer comando"
		time.Sleep(40 * time.Millisecond)
		lines <- "segundo comando"
		close(lines)
	}()

	var got []string
	CoalesceTextLines(lines, 10*time.Millisecond, func(text string) { got = append(got, text) })

	want := []string{"primer comando", "segundo comando"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCoalesceTextLinesKeepsBlankLinesInsideFastBlock(t *testing.T) {
	lines := make(chan string, 3)
	lines <- "uno"
	lines <- ""
	lines <- "tres"
	close(lines)

	var got []string
	CoalesceTextLines(lines, time.Hour, func(text string) { got = append(got, text) })

	if len(got) != 1 || got[0] != "uno\n\ntres" {
		t.Fatalf("got %#v", got)
	}
}
