import { waitFor } from "@testing-library/react"
import React from "react"
import { Redirect } from "./Redirect"
import { render, history } from "../test_helpers"

describe("Redirect", () => {
  // Reset the router to '/' before every test
  beforeEach(() => {
    history.replace("/")
  })

  it("performs client-side redirect on render", async () => {
    // When
    render(<Redirect to="/workspaces/v2" />)

    // Then
    await waitFor(() => {
      expect(history.location.pathname).toEqual("/workspaces/v2")
    })
  })
})
