import type { Meta, StoryObj } from "@storybook/react-vite";
import * as monaco from "monaco-editor";
import { expect, fn, waitFor, within } from "storybook/test";
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
		path: "inmemory://model/with-on-change-handler.tf",
	},
	// Target a deterministic model URI so this story remains stable even when
	// other Monaco stories have active models in the same Storybook session.
	async play({ args, canvasElement }) {
		const canvas = within(canvasElement);
		const editor = canvas.getByRole("textbox");
		const modelURI = monaco.Uri.parse(args.path as string);
		let model = monaco.editor.getModel(modelURI);

		await waitFor(() => {
			model = monaco.editor.getModel(modelURI);
			expect(model).not.toBeNull();
		});

		if (!model) {
			throw new Error("Monaco model is unavailable for WithOnChangeHandler.");
		}

		model.setValue("");

		await expect(editor).toHaveValue("");
		await expect(args.onChange).toHaveBeenCalledOnce();
		await expect(args.onChange).toHaveBeenCalledWith("");

		model.setValue("fnord");

		await expect(editor).toHaveValue("fnord");
		await expect(args.onChange).toHaveBeenCalledTimes(2);
		await expect(args.onChange).toHaveBeenLastCalledWith("fnord");
	},
};
