import type { Meta, StoryObj } from "@storybook/react";
import {
  Badges,
  AlphaBadge,
  EnabledBadge,
  EntitledBadge,
  EnterpriseBadge,
} from "./Badges";

const meta: Meta<typeof Badges> = {
  title: "components/Badges",
  component: Badges,
  args: {},
};

export default meta;
type Story = StoryObj<typeof Badges>;

export const Enabled: Story = {
  args: {
    children: <EnabledBadge />,
  },
};
export const Entitled: Story = {
  args: {
    children: <EntitledBadge />,
  },
};
export const Enterprise: Story = {
  args: {
    children: <EnterpriseBadge />,
  },
};
export const Alpha: Story = {
  args: {
    children: <AlphaBadge />,
  },
};
