import { renderComponent } from "testHelpers/renderHelpers";
import { ResourceCard } from "components/Resources/ResourceCard";
import { MockWorkspaceResource } from "testHelpers/entities";
import { screen } from "@testing-library/react";
import { WorkspaceResourceMetadata } from "api/typesGenerated";

describe("Resource Card", () => {
  it("renders daily cost and metadata tiles", async () => {
    renderComponent(
      <ResourceCard resource={MockWorkspaceResource} agentRow={() => <></>} />,
    );
    expect(
      screen.getByText(MockWorkspaceResource.daily_cost),
    ).toBeInTheDocument();

    expect(
      screen.getByText(MockWorkspaceResource.metadata?.[0].value as string),
    ).toBeInTheDocument();
  });

  it("renders daily cost and 3 metadata tiles", async () => {
    const mockResource = {
      ...MockWorkspaceResource,
      metadata: [
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "18GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "24GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "32GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "48GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "60GB",
        },
      ],
    };

    renderComponent(
      <ResourceCard resource={mockResource} agentRow={() => <></>} />,
    );
    expect(screen.getByText(mockResource.daily_cost)).toBeInTheDocument();
    expect(
      screen.getByText(mockResource.metadata?.[0].value),
    ).toBeInTheDocument();
    expect(
      screen.getByText(mockResource.metadata?.[1].value),
    ).toBeInTheDocument();
    expect(
      screen.getByText(mockResource.metadata?.[2].value),
    ).toBeInTheDocument();
    // last element is hidden
    expect(
      screen.queryByText(mockResource.metadata?.[3].value),
    ).not.toBeInTheDocument();
  });

  it("renders 4 metadata tiles if no daily cost", async () => {
    const mockResource = {
      ...MockWorkspaceResource,
      daily_cost: 0,
      metadata: [
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "18GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "24GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "32GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "48GB",
        },
        {
          ...(MockWorkspaceResource.metadata?.[0] as WorkspaceResourceMetadata),
          value: "60GB",
        },
      ],
    };

    renderComponent(
      <ResourceCard resource={mockResource} agentRow={() => <></>} />,
    );
    expect(screen.queryByText(mockResource.daily_cost)).not.toBeInTheDocument();
    expect(
      screen.getByText(mockResource.metadata?.[0].value),
    ).toBeInTheDocument();
    expect(
      screen.getByText(mockResource.metadata?.[1].value),
    ).toBeInTheDocument();
    expect(
      screen.getByText(mockResource.metadata?.[2].value),
    ).toBeInTheDocument();
    expect(
      screen.getByText(mockResource.metadata?.[3].value),
    ).toBeInTheDocument();
  });
});
