import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { LogLine, LogLinePrefix } from "./LogLine";

const meta: Meta<typeof LogLine> = {
  title: "components/Logs/LogLine",
  parameters: { chromatic },
  component: LogLine,
  args: {
    level: "info",
    children: (
      <>
        <LogLinePrefix>13:45:31.072</LogLinePrefix>
        <span>info: Starting build</span>
      </>
    ),
  },
};

export default meta;

type Story = StoryObj<typeof LogLine>;

export const Info: Story = {};

export const Debug: Story = {
  args: {
    level: "debug",
  },
};

export const Error: Story = {
  args: {
    level: "error",
  },
};

export const Trace: Story = {
  args: {
    level: "trace",
  },
};

export const Warn: Story = {
  args: {
    level: "warn",
  },
};
