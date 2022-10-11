import { waitFor } from "@testing-library/react"
import "jest-canvas-mock"
import WS from "jest-websocket-mock"
import { rest } from "msw"
import { Route, Routes } from "react-router-dom"
import { TextDecoder, TextEncoder } from "util"
import { ReconnectingPTYRequest } from "../../api/types"
import {
  history,
  MockWorkspace,
  MockWorkspaceAgent,
  render,
} from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import TerminalPage, { Language } from "./TerminalPage"

Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: jest.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: jest.fn(), // deprecated
    removeListener: jest.fn(), // deprecated
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    dispatchEvent: jest.fn(),
  })),
})

Object.defineProperty(window, "TextEncoder", {
  value: TextEncoder,
})

const renderTerminal = () => {
  return render(
    <Routes>
      <Route
        path="/:username/:workspace/terminal"
        element={<TerminalPage renderer="dom" />}
      />
    </Routes>,
  )
}

const expectTerminalText = (container: HTMLElement, text: string) => {
  return waitFor(() => {
    const elements = container.getElementsByClassName("xterm-rows")
    if (elements.length === 0) {
      throw new Error("no xterm-rows")
    }
    const row = elements[0] as HTMLDivElement
    if (!row.textContent) {
      throw new Error("no text content")
    }
    expect(row.textContent).toContain(text)
  })
}

describe("TerminalPage", () => {
  beforeEach(() => {
    history.push(`/some-user/${MockWorkspace.name}/terminal`)
  })

  it("shows an error if fetching workspace fails", async () => {
    // Given
    server.use(
      rest.get(
        "/api/v2/users/:userId/workspace/:workspaceName",
        (req, res, ctx) => {
          return res(ctx.status(500), ctx.json({ id: "workspace-id" }))
        },
      ),
    )

    // When
    const { container } = renderTerminal()

    // Then
    await expectTerminalText(container, Language.workspaceErrorMessagePrefix)
  })

  it("shows an error if the websocket fails", async () => {
    // Given
    server.use(
      rest.get("/api/v2/workspaceagents/:agentId/pty", (req, res, ctx) => {
        return res(ctx.status(500), ctx.json({}))
      }),
    )

    // When
    const { container } = renderTerminal()

    // Then
    await expectTerminalText(container, Language.websocketErrorMessagePrefix)
  })

  it("renders data from the backend", async () => {
    // Given
    const server = new WS(
      "ws://localhost/api/v2/workspaceagents/" + MockWorkspaceAgent.id + "/pty",
    )
    const text = "something to render"

    // When
    const { container } = renderTerminal()

    // Then
    await server.connected
    server.send(text)
    await expectTerminalText(container, text)
    server.close()
  })

  it("resizes on connect", async () => {
    // Given
    const server = new WS(
      "ws://localhost/api/v2/workspaceagents/" + MockWorkspaceAgent.id + "/pty",
    )

    // When
    renderTerminal()

    // Then
    await server.connected
    const msg = await server.nextMessage
    const req: ReconnectingPTYRequest = JSON.parse(
      new TextDecoder().decode(msg as Uint8Array),
    )

    expect(req.height).toBeGreaterThan(0)
    expect(req.width).toBeGreaterThan(0)
    server.close()
  })

  it("supports workspace.agent syntax", async () => {
    // Given
    const server = new WS(
      "ws://localhost/api/v2/workspaceagents/" + MockWorkspaceAgent.id + "/pty",
    )
    const text = "something to render"

    // When
    history.push(
      `/some-user/${MockWorkspace.name}.${MockWorkspaceAgent.name}/terminal`,
    )
    const { container } = renderTerminal()

    // Then
    await server.connected
    server.send(text)
    await expectTerminalText(container, text)
    server.close()
  })
})
