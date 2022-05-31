import { inspect } from "@xstate/inspect"
import ReactDOM from "react-dom"
import { Interpreter } from "xstate"
import { App } from "./app"

// if this is a development build and the developer wants to inspect
if (process.env.NODE_ENV === "development" && process.env.INSPECT_XSTATE === "true") {
  // configure the XState inspector to open in a new tab
  inspect({
    url: "https://stately.ai/viz?inspect",
    iframe: false,
  })
  // configure all XServices to use the inspector
  Interpreter.defaultOptions.devTools = true
}

// This is the entry point for the app - where everything start.
// In the future, we'll likely bring in more bootstrapping logic -
// like: https://github.com/coder/m/blob/50898bd4803df7639bd181e484c74ac5d84da474/product/coder/site/pages/_app.tsx#L32
const main = () => {
  console.info(`    ▄█▀    ▀█▄
     ▄▄ ▀▀▀  █▌   ██▀▀█▄          ▐█
 ▄▄██▀▀█▄▄▄  ██  ██      █▀▀█ ▐█▀▀██ ▄█▀▀█ █▀▀
█▌   ▄▌   ▐█ █▌  ▀█▄▄▄█▌ █  █ ▐█  ██ ██▀▀  █
     ██████▀▄█    ▀▀▀▀   ▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀ ▀
`)
  const element = document.getElementById("root")
  ReactDOM.render(<App />, element)
}

main()
