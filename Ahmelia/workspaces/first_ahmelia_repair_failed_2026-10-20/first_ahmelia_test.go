package main

import "testing"

func TestHello(t *testing.T) {
    result := Hello()
    if result != "Hola" {
        t.Errorf("expected Hello() to return "Hola", but got %q", result)
    }
}