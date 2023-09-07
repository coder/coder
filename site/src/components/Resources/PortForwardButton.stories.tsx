import Box from "@mui/material/Box";
import { PortForwardPopoverView } from "./PortForwardButton";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MockListeningPortsResponse,
  MockWorkspaceAgent,
} from "testHelpers/entities";

const meta: Meta<typeof PortForwardPopoverView> = {
  title: "components/PortForwardPopoverView",
  component: PortForwardPopoverView,
  decorators: [
    (Story) => (
      <Box
        sx={{
          width: (theme) => theme.spacing(38),
          border: (theme) => `1px solid ${theme.palette.divider}`,
          borderRadius: 1,
          backgroundColor: (theme) => theme.palette.background.paper,
        }}
      >
        <Story />
      </Box>
    ),
  ],
  args: {
    agent: MockWorkspaceAgent,
  },
};

export default meta;
type Story = StoryObj<typeof PortForwardPopoverView>;

export const WithPorts: Story = {
  args: {
    ports: MockListeningPortsResponse.ports,
  },
};

export const Empty: Story = {
  args: {
    ports: [],
  },
};
