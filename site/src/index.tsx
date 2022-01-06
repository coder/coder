import React from "react"
import ReactDOM from "react-dom"
import CssBaseline from "@material-ui/core/CssBaseline"
import Box from "@material-ui/core/Box"
import Paper from "@material-ui/core/Paper"

import { EmptyState } from "./components/EmptyState"

import { BrowserRouter as Router, Switch, Route } from "react-router-dom"

function render() {
  const element = document.getElementById("root")

  const button = {
    children: "New Workspace",
    onClick: () => alert("Not yet implemented"),
  }

  const component = (
    <>
      <CssBaseline />
      <Router>
        <Switch>
          <Route path="/">
            <Paper style={{ maxWidth: "1380px", margin: "1em auto" }}>
              <Box pt={4} pb={4}>
                <EmptyState message="No workspaces available." button={button} />
              </Box>
            </Paper>
          </Route>
        </Switch>
      </Router>
    </>
  )

  ReactDOM.render(component, element)
}

render()
