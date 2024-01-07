import { type PropsWithChildren } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { Abbr } from "./Abbr";

// Just here to make the abbreviated part more obvious in the component library
const Underline = ({ children }: PropsWithChildren) => (
  <span css={{ textDecoration: "underline dotted" }}>{children}</span>
);

const meta: Meta<typeof Abbr> = {
  title: "components/Abbr",
  component: Abbr,
  decorators: [
    (Story) => (
      <>
        <p>Try the following text out in a screen reader!</p>
        <Story />
      </>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Abbr>;

export const InlinedShorthand: Story = {
  args: {
    pronunciation: "shorthand",
    children: "ms",
    title: "milliseconds",
  },
  decorators: [
    (Story) => (
      <p css={{ maxWidth: "40em" }}>
        The physical pain of getting bonked on the head with a cartoon mallet
        lasts precisely 593{" "}
        <Underline>
          <Story />
        </Underline>
        . The emotional turmoil and complete embarrassment lasts forever.
      </p>
    ),
  ],
};

export const Acronym: Story = {
  args: {
    pronunciation: "acronym",
    children: "NASA",
    title: "National Aeronautics and Space Administration",
  },
  decorators: [
    (Story) => (
      <Underline>
        <Story />
      </Underline>
    ),
  ],
};

export const Initialism: Story = {
  args: {
    pronunciation: "initialism",
    children: "CLI",
    title: "Command-Line Interface",
  },
  decorators: [
    (Story) => (
      <Underline>
        <Story />
      </Underline>
    ),
  ],
};
