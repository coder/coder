import { TemplateVersionExternalAuth } from "api/typesGenerated";
import { ExternalAuthItem } from "./ExternalAuthItem";
import type { Meta, StoryObj } from "@storybook/react";

const MockExternalAuth: TemplateVersionExternalAuth = {
  id: "",
  type: "",
  display_name: "GitHub",
  display_icon: "/icon/github.svg",
  authenticate_url: "",
  authenticated: false,
};

const meta: Meta<typeof ExternalAuthItem> = {
  title: "pages/CreateWorkspacePage/ExternalAuthBanner/ExternalAuthItem",
  component: ExternalAuthItem,
  decorators: [
    (Story) => (
      <div css={{ maxWidth: 390 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ExternalAuthItem>;

export const Default: Story = {
  args: {
    provider: MockExternalAuth,
  },
};

export const Connected: Story = {
  args: {
    provider: {
      ...MockExternalAuth,
      authenticated: true,
    },
  },
};

export const Connecting: Story = {
  args: {
    provider: MockExternalAuth,
    defaultStatus: "connecting",
    isPolling: true,
  },
};
