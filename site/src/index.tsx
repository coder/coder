import React from "react"
import ReactDOM from "react-dom"
import CssBaseline from "@material-ui/core/CssBaseline"
import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"

import { dark } from "./theme"

import ThemeProvider from "@material-ui/styles/ThemeProvider"

import { EmptyState } from "./components/EmptyState"

import { Workspaces } from "./pages"

import { BrowserRouter as Router, Switch, Route } from "react-router-dom"

function render() {
  const element = document.getElementById("root")

  const component = (
    <>
      <ThemeProvider theme={dark}>
        <CssBaseline />
        <Router>
          <Switch>
            <Route path="/">
              <Workspaces />
            </Route>
          </Switch>
        </Router>
      </ThemeProvider>
    </>
  )

  ReactDOM.render(component, element)
}

render()
