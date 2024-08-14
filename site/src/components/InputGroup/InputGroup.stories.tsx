import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import type { Meta, StoryObj } from "@storybook/react";
import { InputGroup } from "./InputGroup";

const meta: Meta<typeof InputGroup> = {
  title: "components/InputGroup",
  component: InputGroup,
};

export default meta;
type Story = StoryObj<typeof InputGroup>;

export const Default: Story = {
  args: {
    children: (
      <>
        <Button>Menu</Button>
        <TextField size="small" placeholder="Search..." />
      </>
    ),
  },
};

export const FocusedTextField: Story = {
  args: {
    children: (
      <>
        <Button>Menu</Button>
        <TextField autoFocus size="small" placeholder="Search..." />
      </>
    ),
  },
};

export const ErroredTextField: Story = {
  args: {
    children: (
      <>
        <Button>Menu</Button>
        <TextField
          error
          size="small"
          placeholder="Search..."
          helperText="Some error message..."
        />
      </>
    ),
  },
};

export const FocusedErroredTextField: Story = {
  args: {
    children: (
      <>
        <Button>Menu</Button>
        <TextField
          autoFocus
          error
          size="small"
          placeholder="Search..."
          helperText="Some error message..."
        />
      </>
    ),
  },
};

export const WithThreeElements: Story = {
  args: {
    children: (
      <>
        <Button>Menu</Button>
        <TextField size="small" placeholder="Search..." />
        <Button>Submit</Button>
      </>
    ),
  },
};
