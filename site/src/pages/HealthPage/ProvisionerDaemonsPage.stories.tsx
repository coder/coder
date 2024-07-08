import type { StoryObj, Meta } from "@storybook/react";
import { ProvisionerDaemonsPage } from "./ProvisionerDaemonsPage";
import { generateMeta } from "./storybook";

const meta: Meta = {
  title: "pages/Health/ProvisionerDaemons",
  ...generateMeta({
    path: "/health/provisioner-daemons",
    element: <ProvisionerDaemonsPage />,
  }),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as ProvisionerDaemons };
