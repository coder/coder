import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../test_helpers"
import { UsersPage } from "./UsersPage"

describe("Users Page", () => {
  it("has a header with the total number of users", async () => {
    render(<UsersPage />)
    const total = await screen.findByText(/\d+ total/)
    expect(total.innerHTML).toEqual("2 total")
  })
  it("shows users", async () => {
    render(<UsersPage />)
    const users = await screen.findAllByText(/.*@coder.com/)
    expect(users.length).toEqual(2)
  })
})
