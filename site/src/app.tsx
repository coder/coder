import React from "react"
import { BrowserRouter as Router, Switch, Route } from "react-router-dom"

import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"

import { dark } from "./theme"
import { Workspaces } from "./pages"

/**
 * <App /> is the root rendering logic of the application - setting up our router
 * and any contexts / global state management.
 * @returns
 */

export const App: React.FC = () => {
  return (
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
  )
}
