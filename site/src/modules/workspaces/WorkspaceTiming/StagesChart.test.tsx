import { render, screen } from "@testing-library/react";
import { StagesChart, agentStages, type Stage } from "./StagesChart";

describe("StagesChart", () => {
  const onSelectStage = jest.fn();
  
  // Mock the stage timings
  const mockStageWithTimings = {
    stage: {
      name: "connect",
      label: "connect",
      section: "agent (test)",
      tooltip: { title: <div>Connect</div> },
    } as Stage,
    visibleResources: 1,
    range: {
      startedAt: new Date("2023-01-01T12:00:00Z"),
      endedAt: new Date("2023-01-01T12:01:00Z"),
    },
  };
  
  // Mock a stage with no timings
  const mockStageWithoutTimings = {
    stage: {
      name: "start",
      label: "run startup scripts",
      section: "agent (test)",
      tooltip: { title: <div>Run startup scripts</div> },
    } as Stage,
    visibleResources: 0,
    range: undefined,
  };
  
  it("should render stages with timing data", () => {
    render(
      <StagesChart 
        timings={[mockStageWithTimings]} 
        onSelectStage={onSelectStage} 
      />
    );
    
    // Should display the section header
    expect(screen.getByText("agent (test)")).toBeInTheDocument();
    
    // Should display the stage label
    expect(screen.getByText("connect")).toBeInTheDocument();
  });
  
  it("should NOT render empty startup scripts stage with no visible resources", () => {
    render(
      <StagesChart 
        timings={[mockStageWithoutTimings]} 
        onSelectStage={onSelectStage} 
      />
    );
    
    // Should display the section header 
    expect(screen.getByText("agent (test)")).toBeInTheDocument();
    
    // Should NOT display the "run startup scripts" label as it has no timing data and no resources
    expect(screen.queryByText("run startup scripts")).not.toBeInTheDocument();
  });
  
  it("should render both stages when the startup script stage has resources", () => {
    const mockStartStageWithResources = {
      ...mockStageWithoutTimings,
      visibleResources: 1, // Has one script
    };
    
    render(
      <StagesChart 
        timings={[mockStageWithTimings, mockStartStageWithResources]} 
        onSelectStage={onSelectStage} 
      />
    );
    
    // Should display both stage labels
    expect(screen.getByText("connect")).toBeInTheDocument();
    expect(screen.getByText("run startup scripts")).toBeInTheDocument();
  });
});