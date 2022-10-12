import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import {
  history,
  MockAuditLog,
  MockAuditLog2,
  MockAuditLogWithEmptyDiff,
  render,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers"
import * as CreateDayString from "util/createDayString"
import AuditPage from "./AuditPage"
import { server } from "testHelpers/server"
import { rest } from "msw"

describe("AuditPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")
  })

  it("shows the audit logs", async () => {
    // When
    render(<AuditPage />)

    // Then
    await screen.findByTestId(`audit-log-row-${MockAuditLog.id}`)
    screen.getByTestId(`audit-log-row-${MockAuditLog2.id}`)
  })

  it("filters out audit logs with empty diffs", async () => {
    server.use(
      rest.get(`/api/v2/audit`, (_, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockAuditLogWithEmptyDiff))
      }),
    )

    // When
    render(<AuditPage />)

    // Then
    const logRow = screen.queryByTestId(`audit-log-row-${MockAuditLogWithEmptyDiff.id}`)
    expect(logRow).not.toBeInTheDocument()
  })

  describe("Filtering", () => {
    it("filters by typing", async () => {
      const getAuditLogsSpy = jest
        .spyOn(API, "getAuditLogs")
        .mockResolvedValue({ audit_logs: [MockAuditLog] })

      render(<AuditPage />)
      await waitForLoaderToBeRemoved()

      // Reset spy so we can focus on the call with the filter
      getAuditLogsSpy.mockReset()

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

    it("filters by URL", async () => {
      const getAuditLogsSpy = jest
        .spyOn(API, "getAuditLogs")
        .mockResolvedValue({ audit_logs: [MockAuditLog] })

      const query = "resource_type:workspace action:create"
      history.push(`/audit?filter=${encodeURIComponent(query)}`)
      render(<AuditPage />)

      await waitForLoaderToBeRemoved()

      expect(getAuditLogsSpy).toBeCalledWith({ limit: 25, offset: 0, q: query })
    })
  })
})
