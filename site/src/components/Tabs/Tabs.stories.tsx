import { Tabs, TabLink } from "./Tabs";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof Tabs> = {
  title: "components/Tabs",
  component: Tabs,
};

export default meta;
type Story = StoryObj<typeof Tabs>;

export const Default: Story = {
  args: {
    children: (
      <>
        <TabLink to="">Tab 1</TabLink>
        <TabLink to="tab-3">Tab 2</TabLink>
        <TabLink to="tab-4">Tab 3</TabLink>
      </>
    ),
  },
};
