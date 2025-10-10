# What is wasm_exec.js?

`wasm_exec.js` is the **JavaScript runtime bridge** that allows Go code compiled
to WebAssembly to interact with the browser. It's provided by the Go team as
part of the standard Go distribution.

**In simple terms**: wasm_exec.js is the runtime that lets Go run in the browser
via WASM. It's the **glue layer** that provides **bidirectional communication**:

1. **WASM → JavaScript**: When Go calls `js.Global().Set("hello", func)`,
   wasm_exec.js makes that function available as `window.hello()` in JavaScript
2. **JavaScript → WASM**: When JavaScript calls `window.hello()`, wasm_exec.js
   translates that into a Go function call

From JavaScript's perspective, you're just calling regular JavaScript functions.
But behind the scenes, wasm_exec.js is marshaling the call into the WASM module,
executing Go code, and returning the result back to JavaScript.

Without wasm_exec.js, your compiled Go WASM code would just be binary data with
no way to execute or interact with the browser.

## Core Responsibilities

### 1. Provides the `Go` class
Creates a JavaScript class that manages the entire Go WASM runtime lifecycle.
This includes initialization, execution, and cleanup.

### 2. Memory Management Bridge
- **Shares memory** between JavaScript and Go using WebAssembly linear memory
- **Type conversion**: Converts between Go types (int64, strings, slices) and
  JavaScript types (numbers, strings, arrays)
- **Reference counting**: Tracks JavaScript objects that Go code holds
  references to, preventing garbage collection

### 3. Implements syscall/js Package
When your Go code uses `syscall/js` to interact with JavaScript:
- `js.Global()` - returns the global window object
- `js.Value.Get()` - gets a JavaScript property
- `js.Value.Set()` - sets a JavaScript property
- `js.Value.Call()` - calls a JavaScript method
- `js.FuncOf()` - wraps Go functions so JS can call them

All of these are implemented in wasm_exec.js via the `importObject.gojs.*`
functions.

### 4. Provides Minimal OS Interface
Since WASM runs in a sandbox with no OS, wasm_exec.js **fakes** OS
functionality that Go expects:
- **Filesystem** (`fs.writeSync`): Console.log output goes here
- **Process info** (`process.pid`, `process.getuid`): Returns dummy values
- **Time** (`runtime.nanotime1`, `runtime.walltime`): Uses browser
  performance APIs
- **Random** (`runtime.getRandomData`): Uses crypto.getRandomValues
- **Timers** (`setTimeout`/`clearTimeout`): Bridges to browser timers

### 5. Entry Point Management
- Passes command-line args (`this.argv`) to Go's main()
- Passes environment variables (`this.env`) to Go
- Handles Go program exit via `runtime.wasmExit`

## How It Works (Simplified)

```
JavaScript Side              |  Go Side (WASM)
----------------------------|---------------------------
1. new Go()                  |
2. WebAssembly.instantiate   |
   - Passes importObject     |  Imports JS functions
3. go.run(instance)          |
                             |  Calls main()
                             |  js.Global().Set("hello", ...)
   Uses importObject to      |
   set window.hello          |
                             |  Go code running...
4. window.hello()            |
   Triggers Go function      |  hello() executes
   via _makeFuncWrapper      |
                             |  Returns result
   Receives result           |
```

## Key Insight

**Without wasm_exec.js, your Go WASM code cannot run.** It's like the C
standard library - it provides the fundamental runtime support that Go expects
from an operating system, but implemented in JavaScript for the browser
environment.

## The importObject

The most important part is `this.importObject`, which contains all the
JavaScript functions that the Go WASM module imports and calls. These are
organized into namespaces:

- `gojs.runtime.*` - Go runtime functions (memory, time, exit)
- `gojs.syscall/js.*` - JavaScript interop (the syscall/js package)

When Go compiles to WASM, it generates import statements for these functions,
and wasm_exec.js provides the implementations.
