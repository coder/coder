import { render, RenderResult } from "@testing-library/react"
import React from "react"
import { MemoryRouter as Router, Route, Routes } from "react-router-dom"
import { RequireAuth } from "../components/Page/RequireAuth"
import { XServiceProvider } from "../xServices/StateContext"
import { MockUser } from "./entities"

type RenderWithAuthResult = RenderResult & { user: typeof MockUser }

export function renderWithAuth(ui: JSX.Element, { route = "/" }: { route?: string } = {}): RenderWithAuthResult {
  const renderResult = render(
    <Router initialEntries={[route]}>
      <XServiceProvider>
        <Routes>
          <Route path={route} element={<RequireAuth>{ui}</RequireAuth>} />
        </Routes>
      </XServiceProvider>
    </Router>,
  )

  return {
    user: MockUser,
    ...renderResult,
  }
}
