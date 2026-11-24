import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { userEvent, within } from "storybook/test";
import { annotatorHostId, mountAnnotator } from "./mountAnnotator";

type DemoPageProps = { picking: boolean };

// A third-party preview fixture. The real overlay isolates its controls
// and styles in a shadow root within this document.
const DemoPage: React.FC<DemoPageProps> = ({ picking }) => {
	const [comment, setComment] = useState<string>();

	useEffect(() => {
		const handle = mountAnnotator({
			document,
			onSubmit: (submission) => setComment(submission.annotations[0]?.comment),
		});
		handle.setPicking(picking);
		return () => handle.destroy();
	}, [picking]);

	return (
		<main className="min-h-[520px] bg-surface-primary p-8 font-sans text-content-primary">
			<h1 className="mb-6 text-xl font-semibold">Example app settings</h1>
			<p className="-mx-8 mb-6 bg-surface-secondary px-8 py-3 text-center text-sm">
				A full-width announcement from the previewed app.
			</p>
			<section className="max-w-md space-y-4">
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Display name</span>
					<input
						className="w-full rounded border border-border-default bg-surface-secondary px-2 py-1"
						defaultValue="Example user"
					/>
				</label>
				<div className="flex gap-2">
					<button
						type="button"
						id="save-example"
						className="rounded bg-surface-tertiary px-3 py-1.5 text-sm"
					>
						Save
					</button>
					<button type="button" className="rounded px-3 py-1.5 text-sm">
						Cancel
					</button>
				</div>
			</section>
			{comment && (
				<p role="status" className="mt-8 text-sm text-content-secondary">
					Comment sent: {comment}
				</p>
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
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: "Save" }),
		);
	},
};

export const PickingAtEdge: Story = {
	args: { picking: true },
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByText(
				"A full-width announcement from the previewed app.",
			),
		);
	},
};

export const PickingAtRightEdge: Story = {
	args: { picking: true },
	render: (args) => (
		<>
			<DemoPage {...args} />
			<button
				type="button"
				id="right-edge-action"
				className="fixed right-0 top-0 rounded bg-surface-tertiary px-3 py-1.5 text-sm"
			>
				Edge action
			</button>
		</>
	),
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: "Edge action" }),
		);
	},
};

function commentBox(): HTMLElement {
	const shadow = document.getElementById(annotatorHostId)?.shadowRoot;
	const dialog = Array.from(shadow?.children ?? []).find(
		(element) => element.getAttribute("role") === "dialog",
	);
	if (!(dialog instanceof HTMLElement)) {
		throw new Error("The annotation dialog did not open");
	}
	return within(dialog).getByRole("textbox", { name: "Annotation comment" });
}

export const CommentPopup: Story = {
	args: { picking: true },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Save" }),
		);
		await userEvent.type(commentBox(), "Make this the primary action");
	},
};

export const SentOnSave: Story = {
	args: { picking: true },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Save" }),
		);
		await userEvent.type(commentBox(), "Use the primary style{enter}");
	},
};
