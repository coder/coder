import type { Meta, StoryObj } from "@storybook/react";
import { Margins } from "./Margins";

const meta: Meta<typeof Margins> = {
  title: "components/Margins",
  component: Margins,
};

export default meta;
type Story = StoryObj<typeof Margins>;

export const Example: Story = {
  args: {
    children: (
      <div style={{ width: "100%", background: "black" }}>
        Here is some content that will not get too wide!
      </div>
    ),
  },
};
