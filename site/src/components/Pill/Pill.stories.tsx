import { Pill, PillSpinner } from "./Pill";
import type { Meta, StoryObj } from "@storybook/react";
import InfoOutlined from "@mui/icons-material/InfoOutlined";

const meta: Meta<typeof Pill> = {
  title: "components/Pill",
  component: Pill,
  args: {
    children: "Default",
  },
};

export default meta;
type Story = StoryObj<typeof Pill>;

export const Default: Story = {};

export const Danger: Story = {
  args: {
    children: "Danger",
    color: "danger",
  },
};

export const Error: Story = {
  args: {
    children: "Error",
    color: "error",
  },
};

export const Warning: Story = {
  args: {
    children: "Warning",
    color: "warning",
  },
};

export const Notice: Story = {
  args: {
    children: "Notice",
    color: "notice",
  },
};

export const Info: Story = {
  args: {
    children: "Information",
    color: "info",
  },
};

export const Success: Story = {
  args: {
    children: "Success",
    color: "success",
  },
};

export const Active: Story = {
  args: {
    children: "Active",
    color: "active",
  },
};

export const WithIcon: Story = {
  args: {
    children: "Information",
    color: "info",
    icon: <InfoOutlined />,
  },
};

export const WithSpinner: Story = {
  args: {
    icon: <PillSpinner />,
  },
  parameters: {
    chromatic: { delay: 700 },
  },
};
