import InfoOutlined from "@mui/icons-material/InfoOutlined";
import type { Meta, StoryObj } from "@storybook/react";
import { Pill, PillSpinner } from "./Pill";

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
    type: "danger",
  },
};

export const Error: Story = {
  args: {
    children: "Error",
    type: "error",
  },
};

export const Warning: Story = {
  args: {
    children: "Warning",
    type: "warning",
  },
};

export const Notice: Story = {
  args: {
    children: "Notice",
    type: "notice",
  },
};

export const Info: Story = {
  args: {
    children: "Information",
    type: "info",
  },
};

export const Success: Story = {
  args: {
    children: "Success",
    type: "success",
  },
};

export const Active: Story = {
  args: {
    children: "Active",
    type: "active",
  },
};

export const WithIcon: Story = {
  args: {
    children: "Information",
    type: "info",
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
