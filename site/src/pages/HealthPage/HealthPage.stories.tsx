import { Meta, StoryObj } from "@storybook/react";
import { HealthPageView } from "./HealthPage";
import { MockHealth } from "testHelpers/entities";

const meta: Meta<typeof HealthPageView> = {
  title: "pages/HealthPage",
  component: HealthPageView,
  args: {
    tab: {
      value: "derp",
      set: () => {},
    },
    healthStatus: MockHealth,
  },
};

export default meta;
type Story = StoryObj<typeof HealthPageView>;

export const Example: Story = {};

export const AccessURLUnhealthy: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: false,
      severity: "error",
      access_url: {
        ...MockHealth.access_url,
        healthy: false,
        error: "ouch",
      },
    },
  },
};

export const AccessURLWarning: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: true,
      severity: "warning",
      access_url: {
        ...MockHealth.access_url,
        healthy: true,
        warnings: ["foobar"],
      },
    },
  },
};

export const DatabaseUnhealthy: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: false,
      severity: "error",
      database: {
        ...MockHealth.database,
        healthy: false,
        error: "ouch",
      },
    },
  },
};

export const DatabaseWarning: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: true,
      severity: "warning",
      database: {
        ...MockHealth.database,
        healthy: true,
        warnings: ["foobar"],
      },
    },
  },
};

export const WebsocketUnhealthy: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: false,
      severity: "error",
      websocket: {
        ...MockHealth.websocket,
        healthy: false,
        error: "ouch",
      },
    },
  },
};

export const WebsocketWarning: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: true,
      severity: "warning",
      websocket: {
        ...MockHealth.websocket,
        healthy: true,
        warnings: ["foobar"],
      },
    },
  },
};

export const UnhealthyDERP: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: false,
      severity: "error",
      derp: {
        ...MockHealth.derp,
        healthy: false,
        error: "ouch",
      },
    },
  },
};

export const DERPWarnings: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      severity: "warning",
      derp: {
        ...MockHealth.derp,
        warnings: ["foobar"],
      },
    },
  },
};

export const ProxyUnhealthy: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      severity: "error",
      healthy: false,
      workspace_proxy: {
        ...MockHealth.workspace_proxy,
        healthy: false,
        error: "ouch",
      },
    },
  },
};

export const ProxyWarning: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      severity: "warning",
      workspace_proxy: {
        ...MockHealth.workspace_proxy,
        warnings: ["foobar"],
      },
    },
  },
};
