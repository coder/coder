import Button from "@mui/material/Button";
import { mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react";
import { ErrorAlert } from "./ErrorAlert";

const mockError = mockApiError({
  message: "Email or password was invalid",
  detail: "Password is invalid",
});

const meta: Meta<typeof ErrorAlert> = {
  title: "components/ErrorAlert",
  component: ErrorAlert,
  args: {
    error: mockError,
    dismissible: false,
  },
};

export default meta;
type Story = StoryObj<typeof ErrorAlert>;

const ExampleAction = (
  <Button onClick={() => null} size="small" variant="text">
    Button
  </Button>
);

export const WithOnlyMessage: Story = {
  args: {
    error: mockApiError({
      message: "Email or password was invalid",
    }),
  },
};

export const WithDismiss: Story = {
  args: {
    dismissible: true,
  },
};

export const WithAction: Story = {
  args: {
    actions: [ExampleAction],
  },
};

export const WithActionAndDismiss: Story = {
  args: {
    actions: [ExampleAction],
    dismissible: true,
  },
};

export const WithNonApiError: Story = {
  args: {
    error: new Error("Non API error here"),
  },
};
