import type { StoryObj, Meta } from "@storybook/react";
import { DatabasePage } from "./DatabasePage";
import { generateMeta } from "./storybook";

const meta: Meta = {
  title: "pages/Health/Database",
  ...generateMeta({
    path: "/health/database",
    element: <DatabasePage />,
  }),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as Database };
