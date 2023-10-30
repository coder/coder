import { waitFor } from "@testing-library/react";
import "jest-canvas-mock";
import WS from "jest-websocket-mock";
import { rest } from "msw";
import {
  MockUser,
  MockWorkspace,
  MockWorkspaceAgent,
} from "testHelpers/entities";
import { TextDecoder, TextEncoder } from "util";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import TerminalPage, { Language } from "./TerminalPage";
import * as API from "api/api";

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
});

Object.defineProperty(window, "TextEncoder", {
  value: TextEncoder,
});

const renderTerminal = async (
  route = `/${MockUser.username}/${MockWorkspace.name}/terminal`,
) => {
  const utils = renderWithAuth(<TerminalPage />, {
    route,
    path: "/:username/:workspace/terminal",
  });
  await waitForLoaderToBeRemoved();
  return utils;
};

const expectTerminalText = (container: HTMLElement, text: string) => {
  return waitFor(
    () => {
      const elements = container.getElementsByClassName("xterm-rows");
      if (elements.length === 0) {
        throw new Error("no xterm-rows");
      }
      const row = elements[0] as HTMLDivElement;
      if (!row.textContent) {
        throw new Error("no text content");
      }
      expect(row.textContent).toContain(text);
    },
    { timeout: 5_000 },
  );
};

describe("TerminalPage", () => {
  it("loads the right workspace data", async () => {
    const spy = jest
      .spyOn(API, "getWorkspaceByOwnerAndName")
      .mockResolvedValue(MockWorkspace);
    const ws = new WS(
      `ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
    );
    await renderTerminal(
      `/${MockUser.username}/${MockWorkspace.name}/terminal`,
    );
    await waitFor(() => {
      expect(API.getWorkspaceByOwnerAndName).toHaveBeenCalledWith(
        MockUser.username,
        MockWorkspace.name,
      );
    });
    spy.mockRestore();
    ws.close();
  });

  it("shows an error if fetching workspace fails", async () => {
    // Given
    server.use(
      rest.get(
        "/api/v2/users/:userId/workspace/:workspaceName",
        (req, res, ctx) => {
          return res(ctx.status(500), ctx.json({ id: "workspace-id" }));
        },
      ),
    );

    // When
    const { container } = await renderTerminal();

    // Then
    await expectTerminalText(container, Language.workspaceErrorMessagePrefix);
  });

  it("shows an error if the websocket fails", async () => {
    // Given
    server.use(
      rest.get("/api/v2/workspaceagents/:agentId/pty", (req, res, ctx) => {
        return res(ctx.status(500), ctx.json({}));
      }),
    );

    // When
    const { container } = await renderTerminal();

    // Then
    await expectTerminalText(container, Language.websocketErrorMessagePrefix);
  });

  it("renders data from the backend", async () => {
    // Given
    const ws = new WS(
      `ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
    );
    const text = "something to render";

    // When
    const { container } = await renderTerminal();

    // Then
    // Ideally we could use ws.connected but that seems to pause React updates.
    // For now, wait for the initial resize message instead.
    await ws.nextMessage;
    ws.send(text);
    await expectTerminalText(container, text);
    ws.close();
  });

  // Ideally we could just pass the correct size in the web socket URL without
  // having to resize separately afterward (and then we could delete also this
  // test), but we need the initial resize message to have something to wait for
  // in the other tests since ws.connected appears to pause React updates.  So
  // for now the initial resize message (and this test) are here to stay.
  it("resizes on connect", async () => {
    // Given
    const ws = new WS(
      `ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
    );

    // When
    await renderTerminal();

    // Then
    const msg = await ws.nextMessage;
    const req = JSON.parse(new TextDecoder().decode(msg as Uint8Array));
    expect(req.height).toBeGreaterThan(0);
    expect(req.width).toBeGreaterThan(0);
    ws.close();
  });

  it("supports workspace.agent syntax", async () => {
    // Given
    const ws = new WS(
      `ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
    );
    const text = "something to render";

    // When
    const { container } = await renderTerminal(
      `/some-user/${MockWorkspace.name}.${MockWorkspaceAgent.name}/terminal`,
    );

    // Then
    // Ideally we could use ws.connected but that seems to pause React updates.
    // For now, wait for the initial resize message instead.
    await ws.nextMessage;
    ws.send(text);
    await expectTerminalText(container, text);
    ws.close();
  });
});
