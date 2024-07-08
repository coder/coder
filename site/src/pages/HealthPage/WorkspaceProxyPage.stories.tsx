import type { StoryObj } from "@storybook/react";
import { HEALTH_QUERY_KEY } from "api/queries/debug";
import type { HealthcheckReport } from "api/typesGenerated";
import { MockHealth } from "testHelpers/entities";
import { generateMeta } from "./storybook";
import { WorkspaceProxyPage } from "./WorkspaceProxyPage";

const meta = {
  title: "pages/Health/WorkspaceProxy",
  ...generateMeta({
    path: "/health/workspace-proxy",
    element: <WorkspaceProxyPage />,
  }),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

const settingsWithError: HealthcheckReport = {
  ...MockHealth,
  severity: "error",
  workspace_proxy: {
    ...MockHealth.workspace_proxy,
    severity: "error",
    error:
      'EACS03: get healthz endpoint: Get "https://localhost:7080/healthz": http: server gave HTTP response to HTTPS client',
  },
};

export const WithError: Story = {
  parameters: {
    queries: [
      ...meta.parameters.queries,
      {
        key: HEALTH_QUERY_KEY,
        data: settingsWithError,
      },
    ],
  },
};

export { Example as WorkspaceProxy };
