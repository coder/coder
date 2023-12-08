import { StoryObj, Meta } from "@storybook/react";
import { DERPPage } from "./DERPPage";
import { generateMeta } from "./storybook";

const meta: Meta = {
  title: "pages/Health/DERP",
  ...generateMeta({
    path: "/health/derp",
    element: <DERPPage />,
  }),
};

export default meta;
type Story = StoryObj;

export const Default: Story = {};
