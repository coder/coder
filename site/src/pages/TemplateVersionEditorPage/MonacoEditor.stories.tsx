import type { Meta, StoryObj } from "@storybook/react-vite";
import * as monaco from "monaco-editor";
import { expect, fn, waitFor } from "storybook/test";
import { MonacoEditor } from "./MonacoEditor";

const meta: Meta<typeof MonacoEditor> = {
	title: "pages/TemplateVersionEditorPage/MonacoEditor",
	component: MonacoEditor,
	args: {},
	parameters: {
		layout: "fullscreen",
	},
	decorators: [
		(Story) => (
			<div style={{ height: "400px" }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof MonacoEditor>;

export const Empty: Story = {};

export const WithContent: Story = {
	args: {
		value: `terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}
`,
		path: "main.tf",
	},
};

export const WithJSON: Story = {
	args: {
		value: JSON.stringify({ key: "value", nested: { foo: "bar" } }, null, 2),
		path: "config.json",
	},
};

export const WithOnChangeHandler: Story = {
	args: {
		onChange: fn(),
		value: "fnord",
	},
	// Monaco's textarea does not receive or fire events directly. Instead, we
	// have to interact with the editor's model and then assert that the
	// onChange callback was called with the new value.
	async play({ args, canvas }) {
		await waitFor(() => canvas.getByRole("textbox"));

		// there's only one model in the story
		await waitFor(() => expect(monaco.editor.getModels()).toHaveLength(1));

		const model = monaco.editor.getModels()[0];

		model.setValue("");

		await waitFor(async () => {
			await expect(args.onChange).toHaveBeenCalledOnce();
			await expect(args.onChange).toHaveBeenCalledWith("");
		});

		model.setValue("fnord");

		await waitFor(async () => {
			await expect(args.onChange).toHaveBeenCalledTimes(2);
			await expect(args.onChange).toHaveBeenLastCalledWith("fnord");
		});
	},
};
