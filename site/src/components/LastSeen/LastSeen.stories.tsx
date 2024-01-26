import type { Meta, StoryObj } from "@storybook/react";
import dayjs from "dayjs";
import { LastSeen } from "./LastSeen";

const meta: Meta<typeof LastSeen> = {
  title: "components/LastSeen",
  component: LastSeen,
  args: {
    // We typically want this component to be excluded from Chromatic's snapshots,
    // because it creates a lot of noise when a static dates roles over from eg.
    // "2 months ago" to "3 months ago", but these stories use relative dates,
    // and test specific cases that we want to be validated.
    "data-chromatic": "",
  },
};

export default meta;
type Story = StoryObj<typeof LastSeen>;

export const Now: Story = {
  args: {
    at: dayjs(),
  },
};

export const OneDayAgo: Story = {
  args: {
    at: dayjs().subtract(1, "day"),
  },
};

export const OneWeekAgo: Story = {
  args: {
    at: dayjs().subtract(1, "week"),
  },
};

export const OneMonthAgo: Story = {
  args: {
    at: dayjs().subtract(1, "month"),
  },
};

export const OneYearAgo: Story = {
  args: {
    at: dayjs().subtract(1, "year"),
  },
};

export const Never: Story = {
  args: {
    at: dayjs().subtract(101, "year"),
  },
};
