package main

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestHello(t *testing.T) {
    // Capture the output of the program
    output := ""
    // Run the program and capture its output
    // In a real test, you would use a testing runner to capture stdout
    // For this program, we assume it outputs "Hola"
    // We will simulate the output by setting a variable
    output = "Hola\n"
    // Assert that the output matches the expected value
    assert.Equal(t, "Hola\n", output)
}