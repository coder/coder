import Button from "@mui/material/Button";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC, useState } from "react";
import type { WorkspaceAgent, WorkspaceResource } from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
import { ResourceCard } from "./ResourceCard";

const countAgents = (resource: WorkspaceResource) => {
  return resource.agents ? resource.agents.length : 0;
};

interface ResourcesProps {
  resources: WorkspaceResource[];
  agentRow: (agent: WorkspaceAgent, numberOfAgents: number) => JSX.Element;
}

export const Resources: FC<ResourcesProps> = ({ resources, agentRow }) => {
  const theme = useTheme();
  const [shouldDisplayHideResources, setShouldDisplayHideResources] =
    useState(false);
  const displayResources = shouldDisplayHideResources
    ? resources
    : resources
        .filter((resource) => !resource.hide)
        // Display the resources with agents first
        .sort((a, b) => countAgents(b) - countAgents(a));
  const hasHideResources = resources.some((r) => r.hide);

  return (
    <Stack
      direction="column"
      spacing={0}
      css={{ background: theme.palette.background.default }}
    >
      {displayResources.map((resource) => (
        <ResourceCard
          key={resource.id}
          resource={resource}
          agentRow={(agent) => agentRow(agent, countAgents(resource))}
        />
      ))}
      {hasHideResources && (
        <div css={styles.buttonWrapper}>
          <Button
            css={styles.showMoreButton}
            size="small"
            onClick={() => setShouldDisplayHideResources((v) => !v)}
          >
            {shouldDisplayHideResources ? "Hide" : "Show hidden"} resources
            <DropdownArrow close={shouldDisplayHideResources} />
          </Button>
        </div>
      )}
    </Stack>
  );
};

const styles = {
  buttonWrapper: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    marginTop: 16,
  },

  showMoreButton: {
    borderRadius: 9999,
    width: "100%",
    maxWidth: 260,
  },
} satisfies Record<string, Interpolation<Theme>>;
