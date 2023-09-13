import TextField from "@mui/material/TextField";
import { action } from "@storybook/addon-actions";
import { FormFooter } from "../FormFooter/FormFooter";
import { Stack } from "../Stack/Stack";
import { FullPageForm, FullPageFormProps } from "./FullPageForm";
import type { Meta, StoryObj } from "@storybook/react";

const Template = (props: FullPageFormProps) => (
  <FullPageForm {...props}>
    <form
      onSubmit={(e) => {
        e.preventDefault();
      }}
    >
      <Stack>
        <TextField fullWidth label="Field 1" name="field1" />
        <TextField fullWidth label="Field 2" name="field2" />
        <FormFooter isLoading={false} onCancel={action("cancel")} />
      </Stack>
    </form>
  </FullPageForm>
);

const meta: Meta<typeof FullPageForm> = {
  title: "components/FullPageForm",
  component: Template,
};

export default meta;
type Story = StoryObj<typeof FullPageForm>;

export const Example: Story = {
  args: {
    title: "My Form",
    detail: "Lorem ipsum dolor",
  },
};
