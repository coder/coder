import { Pill } from "./Pill";
import type { Meta, StoryObj } from "@storybook/react";
import InfoOutlined from "@mui/icons-material/InfoOutlined";

const meta: Meta<typeof Pill> = {
  title: "components/Pill",
  component: Pill,
};

export default meta;
type Story = StoryObj<typeof Pill>;

export const Default: Story = {
  args: {
    children: "Default",
  },
};

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
