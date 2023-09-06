import TextField from "@mui/material/TextField";
import { action } from "@storybook/addon-actions";
import { ComponentMeta, Story } from "@storybook/react";
import { FormFooter } from "../FormFooter/FormFooter";
import { Stack } from "../Stack/Stack";
import { FullPageForm, FullPageFormProps } from "./FullPageForm";

export default {
  title: "components/FullPageForm",
  component: FullPageForm,
} as ComponentMeta<typeof FullPageForm>;

const Template: Story<FullPageFormProps> = (args) => (
  <FullPageForm {...args}>
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

export const Example = Template.bind({});
Example.args = {
  title: "My Form",
  detail: "Lorem ipsum dolor",
};
