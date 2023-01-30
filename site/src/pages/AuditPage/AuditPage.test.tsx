import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { rest } from "msw"
import {
  renderWithAuth,
  MockAuditLog,
  MockAuditLog2,
  waitForLoaderToBeRemoved,
  MockEntitlementsWithAuditLog,
} from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"

import * as CreateDayString from "util/createDayString"
import AuditPage from "./AuditPage"

interface RenderPageOptions {
  filter?: string
  page?: number
}

const renderPage = async ({ filter, page }: RenderPageOptions = {}) => {
  let route = "/audit"
  const params = new URLSearchParams()

  if (filter) {
    params.set("filter", filter)
  }

  if (page) {
    params.set("page", page.toString())
  }

  if (Array.from(params).length > 0) {
    route += `?${params.toString()}`
  }

  renderWithAuth(<AuditPage />, {
    route,
    path: "/audit",
  })
  await waitForLoaderToBeRemoved()
}

describe("AuditPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")

    // Mock the entitlements
    server.use(
      rest.get("/api/v2/entitlements", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockEntitlementsWithAuditLog))
      }),
    )
  })

  it("shows the audit logs", async () => {
    // When
    await renderPage()

    // Then
    await screen.findByTestId(`audit-log-row-${MockAuditLog.id}`)
    screen.getByTestId(`audit-log-row-${MockAuditLog2.id}`)
  })

  describe("Filtering", () => {
    it("filters by typing", async () => {
      await renderPage()
      await screen.findByText("updated", { exact: false })

      const filterField = screen.getByLabelText("Filter")
      const query = "resource_type:workspace action:create"
      await userEvent.type(filterField, query)
      await screen.findByText("created", { exact: false })
      const editWorkspace = screen.queryByText("updated", { exact: false })
      expect(editWorkspace).not.toBeInTheDocument()
    })

    it("filters by URL", async () => {
      const getAuditLogsSpy = jest
        .spyOn(API, "getAuditLogs")
        .mockResolvedValue({ audit_logs: [MockAuditLog], count: 1 })

      const query = "resource_type:workspace action:create"
      await renderPage({ filter: query })

      expect(getAuditLogsSpy).toBeCalledWith({ limit: 25, offset: 0, q: query })
    })

    it("resets page to 1 when filter is changed", async () => {
      await renderPage({ page: 2 })

      const getAuditLogsSpy = jest.spyOn(API, "getAuditLogs")

      const filterField = screen.getByLabelText("Filter")
      const query = "resource_type:workspace action:create"
      await userEvent.type(filterField, query)

      await waitFor(() =>
        expect(getAuditLogsSpy).toBeCalledWith({
          limit: 25,
          offset: 0,
          q: query,
        }),
      )
    })
  })
})
