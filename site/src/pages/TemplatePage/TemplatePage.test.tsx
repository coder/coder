import { fireEvent, screen, waitFor } from "@testing-library/react"
import dayjs from "dayjs"
import { rest } from "msw"
import { ResizeObserver } from "resize-observer"
import { server } from "testHelpers/server"
import * as CreateDayString from "util/createDayString"
import {
  MockMemberPermissions,
  MockTemplate,
  MockTemplateVersion,
  MockUser,
  MockWorkspaceResource,
  renderWithAuth,
} from "../../testHelpers/renderHelpers"
import { TemplatePage } from "./TemplatePage"

jest.mock("remark-gfm", () => jest.fn())

Object.defineProperty(window, "ResizeObserver", {
  value: ResizeObserver,
})

describe("TemplatePage", () => {
  it("shows the template name, readme and resources", async () => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")

    renderWithAuth(<TemplatePage />, {
      route: `/templates/${MockTemplate.id}`,
      path: "/templates/:template",
    })
    await screen.findByText(MockTemplate.name)
    screen.getByTestId("markdown")
    screen.getByText(MockWorkspaceResource.name)
    screen.queryAllByText(`${MockTemplateVersion.name}`).length
  })
  it("allows an admin to delete a template", async () => {
    renderWithAuth(<TemplatePage />, {
      route: `/templates/${MockTemplate.id}`,
      path: "/templates/:template",
    })
    const dropdownButton = await screen.findByLabelText("open-dropdown")
    fireEvent.click(dropdownButton)
    const deleteButton = await screen.findByText("Delete")
    expect(deleteButton).toBeDefined()
  })
  it("does not allow a member to delete a template", () => {
    // get member-level permissions
    server.use(
      rest.post(`/api/v2/users/${MockUser.id}/authorization`, async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockMemberPermissions))
      }),
    )
    renderWithAuth(<TemplatePage />, {
      route: `/templates/${MockTemplate.id}`,
      path: "/templates/:template",
    })
    const dropdownButton = screen.queryByLabelText("open-dropdown")
    expect(dropdownButton).toBe(null)
  })
  it("automatically reflects updates", async () => {
    renderWithAuth(<TemplatePage />, {
      route: `/templates/${MockTemplate.id}`,
      path: "/templates/:template",
    })
    const lastUpdated = screen.queryByText("a minute ago")
    expect(lastUpdated).toBeNull()
    server.use(
      rest.get(`/api/v2/templates/${MockTemplate.id}/authorization`, async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json({ ...MockTemplate, updated_at: dayjs() }))
      }),
    )
    await waitFor(() => {
      const lastUpdated2 = screen.getByText("a minute ago")
      expect(lastUpdated2).not.toBeNull()
    })
  })
})
