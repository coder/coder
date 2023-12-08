import { StoryObj, Meta } from "@storybook/react";
import { AccessURLPage } from "./AccessURLPage";
import { generateMeta } from "./storybook";

const meta: Meta = {
  title: "pages/Health/AccessURL",
  ...generateMeta({
    path: "/health/access-url",
    element: <AccessURLPage />,
  }),
};

export default meta;
type Story = StoryObj;

export const Default: Story = {};
