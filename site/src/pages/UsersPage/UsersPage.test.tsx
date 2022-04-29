import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../testHelpers"
import { UsersPage } from "./UsersPage"

describe("Users Page", () => {
  it("shows users", async () => {
    render(<UsersPage />)
    const users = await screen.findAllByText(/.*@coder.com/)
    expect(users.length).toEqual(2)
  })
})
