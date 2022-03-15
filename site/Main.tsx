import React from "react"
import ReactDOM from "react-dom"

import { App } from "./app"

// This is the entry point for the app - where everything start.
// In the future, we'll likely bring in more bootstrapping logic -
// like: https://github.com/coder/m/blob/50898bd4803df7639bd181e484c74ac5d84da474/product/coder/site/pages/_app.tsx#L32
const main = () => {
  const element = document.getElementById("root")
  ReactDOM.render(<App />, element)
}

main()
