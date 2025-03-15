import { render, screen } from "@testing-library/react";
import type { 
  AgentConnectionTiming, 
  AgentScriptTiming, 
  ProvisionerTiming 
} from "api/typesGenerated";
import { WorkspaceTimings } from "./WorkspaceTimings";

describe("WorkspaceTimings", () => {
  const mockProvisionerTimings: ProvisionerTiming[] = [
    {
      action: "create",
      applied_at: "2023-01-01T12:00:00Z",
      created_at: "2023-01-01T12:00:00Z",
      ended_at: "2023-01-01T12:01:00Z",
      log_source_id: "1",
      log_url: "",
      resource: "aws_instance.test",
      source: "terraform",
      stage: "apply",
      started_at: "2023-01-01T12:00:00Z",
      status: "ok",
      workspace_build_id: "1",
      workspace_transition: "start",
    },
  ];

  const mockAgentConnectionTimings: AgentConnectionTiming[] = [
    {
      created_at: "2023-01-01T12:01:00Z",
      ended_at: "2023-01-01T12:02:00Z",
      started_at: "2023-01-01T12:01:00Z",
      stage: "connect",
      status: "ok",
      workspace_agent_id: "1",
      workspace_agent_name: "test",
      workspace_build_id: "1",
      workspace_transition: "start",
    },
  ];

  const mockAgentScriptTimings: AgentScriptTiming[] = [
    {
      created_at: "2023-01-01T12:02:00Z",
      display_name: "test script",
      ended_at: "2023-01-01T12:03:00Z",
      exit_code: 0,
      script_id: "1",
      started_at: "2023-01-01T12:02:00Z",
      stage: "start",
      status: "ok",
      workspace_agent_id: "1",
      workspace_build_id: "1",
      workspace_transition: "start",
    },
  ];

  it("renders with all timings", () => {
    render(
      <WorkspaceTimings
        provisionerTimings={mockProvisionerTimings}
        agentConnectionTimings={mockAgentConnectionTimings}
        agentScriptTimings={mockAgentScriptTimings}
        defaultIsOpen={true}
      />
    );

    expect(screen.getByText("Build timeline")).toBeInTheDocument();
  });

  it("renders correctly with empty agent script timings", () => {
    render(
      <WorkspaceTimings
        provisionerTimings={mockProvisionerTimings}
        agentConnectionTimings={mockAgentConnectionTimings}
        agentScriptTimings={[]} // No startup scripts configured
        defaultIsOpen={true}
      />
    );

    expect(screen.getByText("Build timeline")).toBeInTheDocument();
    // Should not show loading state with Skeleton component
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    // Should not show "run startup scripts" stage
    expect(screen.queryByText("run startup scripts")).not.toBeInTheDocument();
  });

  it("shows loading state when provisioner timings are missing", () => {
    render(
      <WorkspaceTimings
        provisionerTimings={[]} // Missing provisioner timings
        agentConnectionTimings={mockAgentConnectionTimings}
        agentScriptTimings={mockAgentScriptTimings}
        defaultIsOpen={true}
      />
    );

    expect(screen.getByText("Build timeline")).toBeInTheDocument();
    // Should be in loading state
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });
});