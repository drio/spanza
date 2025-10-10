package main

import (
	"fmt"
	"syscall/js"
)

// main is the entry point for the WASM module.
// It runs when the WASM module is loaded in the browser.
func main() {
	fmt.Println("Spanza WASM module loaded!")

	// Expose a function called "hello" to JavaScript
	// When JavaScript calls window.hello(), this Go function will execute
	js.Global().Set("hello", js.FuncOf(hello))

	// Keep the Go program running forever
	// Without this, the WASM module would exit immediately
	// and our exposed functions would disappear
	<-make(chan struct{})
}

// hello is a simple function that JavaScript can call
// It takes no arguments and returns a string
func hello(this js.Value, args []js.Value) interface{} {
	message := "Hello from Spanza WASM!"
	fmt.Println(message) // This prints to browser console
	return message       // This returns to JavaScript caller
}
