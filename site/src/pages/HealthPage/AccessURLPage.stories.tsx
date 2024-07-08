import type { StoryObj } from "@storybook/react";
import { HEALTH_QUERY_KEY } from "api/queries/debug";
import type { HealthcheckReport } from "api/typesGenerated";
import { MockHealth } from "testHelpers/entities";
import { AccessURLPage } from "./AccessURLPage";
import { generateMeta } from "./storybook";

const meta = {
  title: "pages/Health/AccessURL",
  ...generateMeta({
    path: "/health/access-url",
    element: <AccessURLPage />,
  }),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

const settingsWithError: HealthcheckReport = {
  ...MockHealth,
  severity: "error",
  access_url: {
    ...MockHealth.access_url,
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

export { Example as AccessURL };
