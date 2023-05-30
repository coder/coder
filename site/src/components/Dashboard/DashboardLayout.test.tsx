import { Route, Routes } from "react-router-dom"
import { renderWithAuth } from "testHelpers/renderHelpers"
import { DashboardLayout } from "./DashboardLayout"
import * as API from "api/api"
import { screen } from "@testing-library/react"

test("Show the new Coder version notification", async () => {
  jest.spyOn(API, "getUpdateCheck").mockResolvedValue({
    current: false,
    version: "v0.12.9",
    url: "https://github.com/coder/coder/releases/tag/v0.12.9",
  })
  renderWithAuth(
    <Routes>
      <Route element={<DashboardLayout />}>
        <Route element={<h1>Test page</h1>} />
      </Route>
    </Routes>,
  )
  await screen.findByTestId("update-check-snackbar")
})
