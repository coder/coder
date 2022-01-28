import { render, waitFor } from "@testing-library/react"
import singletonRouter from "next/router"
import mockRouter from "next-router-mock"
import React from "react"
import { Redirect } from "./Redirect"

describe("Redirect", () => {
  // Reset the router to '/' before every test
  beforeEach(() => {
    mockRouter.setCurrentUrl("/")
  })

  it("performs client-side redirect on render", async () => {
    // When
    render(<Redirect to="/workspaces/v2" />)

    // Then
    await waitFor(() => {
      expect(singletonRouter).toMatchObject({ asPath: "/workspaces/v2" })
    })
  })
})
