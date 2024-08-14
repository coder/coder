import type { Meta, StoryObj } from "@storybook/react";
import { Tabs, TabLink, TabsList } from "./Tabs";

const meta: Meta<typeof Tabs> = {
  title: "components/Tabs",
  component: Tabs,
};

export default meta;
type Story = StoryObj<typeof Tabs>;

export const Default: Story = {
  args: {
    active: "tab-1",
    children: (
      <TabsList>
        <TabLink value="tab-1" to="">
          Tab 1
        </TabLink>
        <TabLink value="tab-2" to="tab-3">
          Tab 2
        </TabLink>
        <TabLink value="tab-3" to="tab-4">
          Tab 3
        </TabLink>
      </TabsList>
    ),
  },
};
