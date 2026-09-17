import { formatAnnotations } from "@coder/annotator/formatAnnotations";
import {
	annotatorHostId,
	mountAnnotator,
} from "@coder/annotator/mountAnnotator";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useEffect, useState } from "react";
import { userEvent, within } from "storybook/test";

/**
 * Stand-in for a customer app rendered inside the port preview. The
 * annotator from the root `annotator/` package is mounted into this
 * document exactly as the injected script would do it, minus the
 * postMessage bridge.
 */
const DemoPage: FC<{ picking: boolean; highlight?: boolean }> = ({
	picking,
	highlight,
}) => {
	const [output, setOutput] = useState<string>();

	useEffect(() => {
		const handle = mountAnnotator({
			document,
			onSubmit: (submission) => setOutput(formatAnnotations(submission)),
		});
		handle.setPicking(picking);
		if (highlight) {
			handle.setHighlights([
				{
					id: "1",
					selector: '[data-testid="save-button"]',
					url: window.location.href,
				},
				{ id: "2", selector: "h1", url: window.location.href },
			]);
		}
		return () => handle.destroy();
	}, [picking, highlight]);

	return (
		<main className="min-h-[520px] bg-white p-8 font-sans text-neutral-900">
			<header className="mb-6 flex items-center justify-between">
				<h1 className="text-xl font-semibold">Acme Settings</h1>
				<nav className="flex gap-3 text-sm">
					<a className="nav-link" href="#general">
						General
					</a>
					<a className="nav-link" href="#billing">
						Billing
					</a>
				</nav>
			</header>
			<section id="general" className="max-w-md space-y-4">
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Display name</span>
					<input
						className="w-full rounded border border-neutral-300 px-2 py-1"
						defaultValue="Jane Doe"
					/>
				</label>
				<div className="flex gap-2">
					<button
						type="button"
						data-testid="save-button"
						className="rounded bg-neutral-200 px-3 py-1.5 text-sm"
					>
						Save
					</button>
					<button type="button" className="rounded px-3 py-1.5 text-sm">
						Cancel
					</button>
				</div>
			</section>
			{output && (
				<pre className="mt-8 max-h-64 overflow-auto rounded bg-neutral-100 p-3 text-xs">
					{output}
				</pre>
			)}
		</main>
	);
};

const meta = {
	title: "annotator/Annotator",
	component: DemoPage,
	args: { picking: false },
	parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DemoPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Idle: Story = {};

export const Picking: Story = {
	args: { picking: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByTestId("save-button"));
	},
};

export const CommentPopup: Story = {
	args: { picking: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByTestId("save-button"));
		const host = document.getElementById(annotatorHostId);
		const textarea = host?.shadowRoot?.querySelector("textarea");
		if (textarea) {
			await userEvent.type(textarea, "Make this the primary action");
		}
	},
};

export const SentOnSave: Story = {
	args: { picking: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByTestId("save-button"));
		const textarea = document
			.getElementById(annotatorHostId)
			?.shadowRoot?.querySelector("textarea");
		if (textarea) {
			await userEvent.type(textarea, "Use the primary style{enter}");
		}
	},
};

export const AgentWorking: Story = {
	args: { highlight: true },
};
