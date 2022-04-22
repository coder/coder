import { screen } from "@testing-library/react"
import React from "react"
import { MockPager, render } from "../../testHelpers"
import { UsersPage } from "./UsersPage"
import { Language } from "./UsersPageView"

describe("Users Page", () => {
  it("has a header with the total number of users", async () => {
    render(<UsersPage />)
    const total = await screen.findByText(/\d+ total/)
    expect(total.innerHTML).toEqual(Language.pageSubtitle(MockPager))
  })
  it("shows users", async () => {
    render(<UsersPage />)
    const users = await screen.findAllByText(/.*@coder.com/)
    expect(users.length).toEqual(2)
  })
})
