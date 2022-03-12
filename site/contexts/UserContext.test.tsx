import React from "react"
import { SWRConfig } from "swr"
import { screen, waitFor } from "@testing-library/react"
import { User, UserProvider, useUser } from "./UserContext"
import { history, MockUser, render } from "../test_helpers"

namespace Helpers {
  // Helper component that renders out the state of the `useUser` hook.
  // It just renders simple text in the 'error', 'me', and 'loading' states,
  // so that the test can get a peak at the state of the hook.
  const TestComponent: React.FC<{ redirectOnFailure: boolean }> = ({ redirectOnFailure }) => {
    const { me, error } = useUser(redirectOnFailure)

    if (error) {
      return <div>{`Error: ${error.toString()}`}</div>
    }
    if (me) {
      return <div>{`Me: ${me.toString()}`}</div>
    }

    return <div>Loading</div>
  }

  // Helper to render a userContext, and all the scaffolding needed
  // (an SWRConfig as well as a UserPRovider)
  export const renderUserContext = (
    simulatedRequest: () => Promise<User>,
    redirectOnFailure: boolean,
  ): React.ReactElement => {
    return (
      // Set up an SWRConfig that works for testing - we'll simulate a request,
      // and set up the cache to reset every test.
      <SWRConfig
        value={{
          fetcher: simulatedRequest,
          // Reset cache for every test. Without this, requests will be cached between test cases.
          provider: () => new Map(),
        }}
      >
        <UserProvider>
          <TestComponent redirectOnFailure={redirectOnFailure} />
        </UserProvider>
      </SWRConfig>
    )
  }
}

describe("UserContext", () => {
  const failingRequest = () => Promise.reject("Failed to load user")
  const successfulRequest = () => Promise.resolve(MockUser)

  // Reset the router to '/' before every test
  beforeEach(() => {
    history.replace("/")
  })

  it("shouldn't redirect if user fails to load and redirectOnFailure is false", async () => {
    // When
    render(Helpers.renderUserContext(failingRequest, false))

    // Then
    // Verify we get an error message
    await waitFor(() => {
      expect(screen.queryByText("Error:", { exact: false })).toBeDefined()
    })
    // ...and the route should be unchanged
    expect(history.location.pathname).toEqual("/")
    expect(history.location.search).toEqual("")
  })

  it("should redirect if user fails to load and redirectOnFailure is true", async () => {
    // When
    render(Helpers.renderUserContext(failingRequest, true))

    // Then
    // Verify we route to the login page
    await waitFor(() => expect(history.location.pathname).toEqual("/login"))
    await waitFor(() => expect(history.location.search).toEqual("?redirect=%2F"))
  })

  it("should not redirect if user loads and redirectOnFailure is true", async () => {
    // When
    render(Helpers.renderUserContext(successfulRequest, true))

    // Then
    // Verify the user is rendered
    await waitFor(() => {
      expect(screen.queryByText("Me:", { exact: false })).toBeDefined()
    })
    // ...and the route should be unchanged
    expect(history.location.pathname).toEqual("/")
    expect(history.location.search).toEqual("")
  })
})
