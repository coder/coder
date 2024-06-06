import { act, renderHook, waitFor } from "@testing-library/react";
import WS from "jest-websocket-mock";
import { type QueryClient, QueryClientProvider } from "react-query";
import { API } from "api/api";
import * as APIModule from "api/api";
import { agentLogsKey } from "api/queries/workspaces";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { createTestQueryClient } from "testHelpers/renderHelpers";
import { type UseAgentLogsOptions, useAgentLogs } from "./useAgentLogs";

afterEach(() => {
  WS.clean();
});

describe("useAgentLogs", () => {
  it("should not fetch logs if disabled", async () => {
    const queryClient = createTestQueryClient();
    const fetchSpy = jest.spyOn(API, "getWorkspaceAgentLogs");
    const wsSpy = jest.spyOn(APIModule, "watchWorkspaceAgentLogs");
    renderUseAgentLogs(queryClient, {
      workspaceId: MockWorkspace.id,
      agentId: MockWorkspaceAgent.id,
      agentLifeCycleState: "ready",
      enabled: false,
    });
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(wsSpy).not.toHaveBeenCalled();
  });

  it("should return existing logs without network calls", async () => {
    const queryClient = createTestQueryClient();
    queryClient.setQueryData(
      agentLogsKey(MockWorkspace.id, MockWorkspaceAgent.id),
      generateLogs(5),
    );
    const fetchSpy = jest.spyOn(API, "getWorkspaceAgentLogs");
    const wsSpy = jest.spyOn(APIModule, "watchWorkspaceAgentLogs");
    const { result } = renderUseAgentLogs(queryClient, {
      workspaceId: MockWorkspace.id,
      agentId: MockWorkspaceAgent.id,
      agentLifeCycleState: "ready",
    });
    await waitFor(() => {
      expect(result.current).toHaveLength(5);
    });
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(wsSpy).not.toHaveBeenCalled();
  });

  it("should fetch logs when empty and should not connect to WebSocket when not starting", async () => {
    const queryClient = createTestQueryClient();
    const fetchSpy = jest
      .spyOn(API, "getWorkspaceAgentLogs")
      .mockResolvedValueOnce(generateLogs(5));
    const wsSpy = jest.spyOn(APIModule, "watchWorkspaceAgentLogs");
    const { result } = renderUseAgentLogs(queryClient, {
      workspaceId: MockWorkspace.id,
      agentId: MockWorkspaceAgent.id,
      agentLifeCycleState: "ready",
    });
    await waitFor(() => {
      expect(result.current).toHaveLength(5);
    });
    expect(fetchSpy).toHaveBeenCalledWith(MockWorkspaceAgent.id);
    expect(wsSpy).not.toHaveBeenCalled();
  });

  it("should fetch logs and connect to websocket when agent is starting", async () => {
    const queryClient = createTestQueryClient();
    const logs = generateLogs(5);
    const fetchSpy = jest
      .spyOn(API, "getWorkspaceAgentLogs")
      .mockResolvedValueOnce(logs);
    const wsSpy = jest.spyOn(APIModule, "watchWorkspaceAgentLogs");
    new WS(
      `ws://localhost/api/v2/workspaceagents/${
        MockWorkspaceAgent.id
      }/logs?follow&after=${logs[logs.length - 1].id}`,
    );
    const { result } = renderUseAgentLogs(queryClient, {
      workspaceId: MockWorkspace.id,
      agentId: MockWorkspaceAgent.id,
      agentLifeCycleState: "starting",
    });
    await waitFor(() => {
      expect(result.current).toHaveLength(5);
    });
    expect(fetchSpy).toHaveBeenCalledWith(MockWorkspaceAgent.id);
    expect(wsSpy).toHaveBeenCalledWith(MockWorkspaceAgent.id, {
      after: logs[logs.length - 1].id,
      onMessage: expect.any(Function),
      onError: expect.any(Function),
    });
  });

  it("update logs from websocket messages", async () => {
    const queryClient = createTestQueryClient();
    const logs = generateLogs(5);
    jest.spyOn(API, "getWorkspaceAgentLogs").mockResolvedValueOnce(logs);
    const server = new WS(
      `ws://localhost/api/v2/workspaceagents/${
        MockWorkspaceAgent.id
      }/logs?follow&after=${logs[logs.length - 1].id}`,
    );
    const { result } = renderUseAgentLogs(queryClient, {
      workspaceId: MockWorkspace.id,
      agentId: MockWorkspaceAgent.id,
      agentLifeCycleState: "starting",
    });
    await waitFor(() => {
      expect(result.current).toHaveLength(5);
    });
    await server.connected;
    act(() => {
      server.send(JSON.stringify(generateLogs(3)));
    });
    await waitFor(() => {
      expect(result.current).toHaveLength(8);
    });
  });
});

function renderUseAgentLogs(
  queryClient: QueryClient,
  options: UseAgentLogsOptions,
) {
  return renderHook(() => useAgentLogs(options), {
    wrapper: ({ children }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    ),
  });
}

function generateLogs(count: number): WorkspaceAgentLog[] {
  return Array.from({ length: count }, (_, i) => ({
    id: i,
    created_at: new Date().toISOString(),
    level: "info",
    output: `Log ${i}`,
    source_id: "",
  }));
}
