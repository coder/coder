import React from "react"
import { render, screen, waitFor } from "@testing-library/react"
import singletonRouter from "next/router"
import mockRouter from "next-router-mock"
import { AuthenticatedRouter } from "./AuthenticatedRouter"
import * as API from "./../../api"

const mockUser: API.User = {
  id: "test-user-id",
  username: "TestUser",
  email: "test@coder.com",
  created_at: "",
}

describe("AuthenticatedRouter", () => {
  beforeEach(() => {
    mockRouter.setCurrentUrl("/")
  })

  it("renders spinner while loading", async () => {
    // Given
    // `fetchUser` never returns..
    const fetchUser = (): Promise<API.User> =>
      new Promise(() => {
        return
      })

    // When
    render(<AuthenticatedRouter fetchUser={fetchUser} />)

    // Then
    const spinner = await screen.findByRole("progressbar")
    expect(spinner).toBeDefined()
  })

  it("renders children if successful", async () => {
    // Given
    // `fetchUser` returns an actual user - we're logged in
    const fetchUser = (): Promise<API.User> => new Promise((resolve) => resolve(mockUser))

    // When
    render(
      <AuthenticatedRouter fetchUser={fetchUser}>
        <div>Hi!</div>
      </AuthenticatedRouter>,
    )

    // Then
    const helloTextElement = await screen.findByText("Hi!")
    expect(helloTextElement).toBeDefined()
  })

  it("redirects to /login if not successful", async () => {
    // Given
    // `fetchUser` fails, we're not logged in
    const fetchUser = (): Promise<API.User> => new Promise((_resolve, reject) => reject("Sorry you're not logged in"))

    // When
    render(<AuthenticatedRouter fetchUser={fetchUser} />)

    // Then
    // Should redirect to login page
    await waitFor(() => expect(singletonRouter).toMatchObject({ asPath: "/login" }))
  })
})
