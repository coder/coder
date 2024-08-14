import TextField from "@mui/material/TextField";
import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import type { FC } from "react";
import { FormFooter } from "../FormFooter/FormFooter";
import { Stack } from "../Stack/Stack";
import { FullPageForm, type FullPageFormProps } from "./FullPageForm";

const Template: FC<FullPageFormProps> = (props) => (
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

const Example: Story = {
  args: {
    title: "My Form",
    detail: "Lorem ipsum dolor",
  },
};

export { Example as FullPageForm };
