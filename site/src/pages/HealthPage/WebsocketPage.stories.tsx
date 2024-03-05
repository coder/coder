import { StoryObj } from "@storybook/react";
import { WebsocketPage } from "./WebsocketPage";
import { generateMeta } from "./storybook";
import { HEALTH_QUERY_KEY } from "api/queries/debug";
import { HealthcheckReport } from "api/typesGenerated";
import { MockHealth } from "testHelpers/entities";

const meta = {
  title: "pages/Health/Websocket",
  ...generateMeta({
    path: "/health/websocket",
    element: <WebsocketPage />,
  }),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

const settingsWithError: HealthcheckReport = {
  ...MockHealth,
  severity: "error",
  websocket: {
    ...MockHealth.websocket,
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

export { Example as Websocket };
