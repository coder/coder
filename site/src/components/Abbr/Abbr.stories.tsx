import type { Meta, StoryObj } from "@storybook/react";
import { Abbr } from "./Abbr";

const meta: Meta<typeof Abbr> = {
  title: "components/Abbr",
  component: Abbr,
  decorators: [
    (Story) => (
      <>
        <p>Try the following text out in a screen reader!</p>

        {/* Just here to make the abbreviated text part more obvious */}
        <p css={{ textDecoration: "underline dotted" }}>
          <Story />
        </p>
      </>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Abbr>;

export const Abbreviation: Story = {
  args: {
    initialism: false,
    children: "NASA",
    expandedText: "National Aeronautics and Space Administration",
  },
};

export const Initialism: Story = {
  args: {
    initialism: true,
    children: "CLI",
    expandedText: "Command-Line Interface",
  },
};

export const InlinedAbbreviation: Story = {
  args: {
    initialism: false,
    children: "ms",
    expandedText: "milliseconds",
  },
  decorators: [
    (Story) => (
      <p>
        The physical pain of getting bonked on the head with a cartoon mallet
        lasts precisely 593
        <Story />. The emotional turmoil and complete embarrassment lasts
        forever.
      </p>
    ),
  ],
};
