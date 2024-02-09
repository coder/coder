import { TemplateVersionExternalAuth } from "api/typesGenerated";
import { ExternalAuthWall } from "./ExternalAuthWall";
import type { Meta, StoryObj } from "@storybook/react";

const MockExternalAuth: TemplateVersionExternalAuth = {
  id: "",
  type: "",
  display_name: "GitHub",
  display_icon: "/icon/github.svg",
  authenticate_url: "",
  authenticated: false,
};

const meta: Meta<typeof ExternalAuthWall> = {
  title: "pages/CreateWorkspacePage/ExternalAuthWall",
  component: ExternalAuthWall,
};

export default meta;
type Story = StoryObj<typeof ExternalAuthWall>;

export const Default: Story = {
  args: {
    providers: [
      MockExternalAuth,
      {
        ...MockExternalAuth,
        display_name: "Google",
        display_icon: "/icon/google.svg",
        authenticated: true,
      },
    ],
  },
};
