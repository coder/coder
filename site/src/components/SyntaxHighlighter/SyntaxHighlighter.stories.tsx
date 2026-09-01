import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as Monaco from "monaco-editor";
import * as monaco from "monaco-editor";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { SyntaxHighlighter } from "./SyntaxHighlighter";

const original = `resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}
`;

const modified = `resource "coder_agent" "main" {
  os   = "linux"
  arch = "arm64"
}
`;

// The diff editor's gutter menu and occurrence highlighter register delayed
// disposables that throw "AbstractContextKeyService has been disposed" when the
// editor unmounts during a story run. They are irrelevant to model disposal, so
// stories turn them off to keep the runner clean; production keeps the defaults.
const stableTeardownOptions: Monaco.editor.IStandaloneDiffEditorConstructionOptions =
	{
		minimap: { enabled: false },
		renderSideBySide: true,
		readOnly: true,
		renderGutterMenu: false,
		occurrencesHighlight: "off",
	};

const meta: Meta<typeof SyntaxHighlighter> = {
	title: "components/SyntaxHighlighter",
	component: SyntaxHighlighter,
	args: {
		language: "hcl",
		editorProps: { options: stableTeardownOptions },
	},
};

export default meta;
type Story = StoryObj<typeof SyntaxHighlighter>;

export const Plain: Story = {
	args: {
		value: original,
	},
};

export const Diff: Story = {
	args: {
		value: modified,
		compareWith: original,
	},
};

// Reproduces the leak from DEVEX-736: a single SyntaxHighlighter instance that
// stays mounted while a file switches between diff and plain across template
// versions. Each diff editor owns two Monaco models, and they must be disposed
// when the diff goes away. Before the fix the models were only disposed on full
// unmount, so toggling diff -> plain -> diff leaked two models per cycle.
const DiffToggle = () => {
	const [showDiff, setShowDiff] = useState(true);
	return (
		<div>
			<button type="button" onClick={() => setShowDiff((show) => !show)}>
				Toggle diff
			</button>
			<SyntaxHighlighter
				language="hcl"
				value={showDiff ? modified : original}
				compareWith={original}
				editorProps={{ options: stableTeardownOptions }}
			/>
		</div>
	);
};

export const DisposesModelsOnDiffToggle: Story = {
	render: () => <DiffToggle />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: "Toggle diff" });

		// Wait for the diff editor to mount its original + modified models, then
		// record the total as a baseline. Every full toggle cycle must return to
		// this number; growth would mean abandoned models are being retained.
		let baseline = 0;
		await waitFor(() => {
			baseline = monaco.editor.getModels().length;
			expect(baseline).toBeGreaterThanOrEqual(2);
		});

		for (let cycle = 0; cycle < 3; cycle++) {
			// Switch to plain: the diff editor unmounts and must dispose its models.
			await userEvent.click(toggle);
			await waitFor(() =>
				expect(monaco.editor.getModels().length).toBeLessThan(baseline),
			);

			// Switch back to diff: a new diff editor mounts and the total must land
			// back on the baseline rather than climbing.
			await userEvent.click(toggle);
			await waitFor(() =>
				expect(monaco.editor.getModels().length).toBe(baseline),
			);
		}
	},
};
