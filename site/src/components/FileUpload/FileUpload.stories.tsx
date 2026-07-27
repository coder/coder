import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor, within } from "storybook/test";
import { Link } from "#/components/Link/Link";
import { FileUpload } from "./FileUpload";

const meta: Meta<typeof FileUpload> = {
	title: "components/FileUpload",
	component: FileUpload,
	args: {
		title: "Upload template",
		description: (
			<>
				The template has to be a .tar or .zip file. You can also use our{" "}
				<Link href="/starter-templates" showExternalIcon={false}>
					starter templates
				</Link>{" "}
				to getting started with Coder.
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof FileUpload>;

const onUpload = fn<(file: File) => void>();
const onUnsupportedFile = fn<(file: File) => void>();

const dropZoneArgs = {
	isUploading: false,
	onUpload,
	onUnsupportedFile,
	removeLabel: "Remove file",
	extensions: ["tar", "zip"],
};

const getDropZone = (canvasElement: HTMLElement): HTMLElement =>
	within(canvasElement).getByRole("button", { name: /Upload template/ });

const fileTransfer = (file: File): DataTransfer => {
	const dataTransfer = new DataTransfer();
	dataTransfer.items.add(file);
	return dataTransfer;
};

const dispatchDrag = (
	target: HTMLElement,
	type: "dragover" | "dragleave" | "drop",
	init: { dataTransfer: DataTransfer; relatedTarget?: EventTarget | null },
): void => {
	target.dispatchEvent(
		new DragEvent(type, { bubbles: true, cancelable: true, ...init }),
	);
};

// Lets React commit any state update queued by a dispatched drag event, so a
// story can assert that the drag state did not change.
const settle = (element: HTMLElement): Promise<void> => {
	const view = element.ownerDocument.defaultView ?? window;
	return new Promise((resolve) => {
		view.requestAnimationFrame(() =>
			view.requestAnimationFrame(() => resolve()),
		);
	});
};

export const Default: Story = {};

export const Uploading: Story = {
	args: {
		isUploading: true,
	},
};

export const WithFile: Story = {
	args: {
		file: new File([], "template.zip"),
	},
};

// Ends while a file is hovering so the pixel snapshot captures the highlight.
export const DragActiveHighlight: Story = {
	args: dropZoneArgs,
	play: async ({ canvasElement }) => {
		const dropZone = getDropZone(canvasElement);
		const idleStyle = getComputedStyle(dropZone);
		const idleBackground = idleStyle.backgroundColor;
		const idleBorder = idleStyle.borderColor;

		dispatchDrag(dropZone, "dragover", {
			dataTransfer: fileTransfer(new File([], "template.zip")),
		});

		await waitFor(() =>
			expect(dropZone).toHaveAttribute("data-drag-active", "true"),
		);
		await waitFor(() => {
			const activeStyle = getComputedStyle(dropZone);
			expect(activeStyle.backgroundColor).not.toBe(idleBackground);
			expect(activeStyle.borderColor).not.toBe(idleBorder);
		});
	},
};

export const DragActiveStateMachine: Story = {
	args: dropZoneArgs,
	play: async ({ canvasElement }) => {
		const dropZone = getDropZone(canvasElement);
		const descendant = dropZone.firstElementChild;
		if (!(descendant instanceof HTMLElement)) {
			throw new Error("Expected the drop zone to render its contents");
		}

		const textTransfer = new DataTransfer();
		textTransfer.setData("text/plain", "https://example.com");
		dispatchDrag(dropZone, "dragover", { dataTransfer: textTransfer });
		await settle(dropZone);
		await expect(dropZone).toHaveAttribute("data-drag-active", "false");

		const dataTransfer = fileTransfer(new File([], "template.zip"));
		dispatchDrag(dropZone, "dragover", { dataTransfer });
		await waitFor(() =>
			expect(dropZone).toHaveAttribute("data-drag-active", "true"),
		);

		dispatchDrag(dropZone, "dragleave", {
			dataTransfer,
			relatedTarget: descendant,
		});
		await settle(dropZone);
		await expect(dropZone).toHaveAttribute("data-drag-active", "true");

		dispatchDrag(dropZone, "dragleave", {
			dataTransfer,
			relatedTarget: canvasElement.ownerDocument.body,
		});
		await waitFor(() =>
			expect(dropZone).toHaveAttribute("data-drag-active", "false"),
		);
	},
};

export const DropUnsupportedFile: Story = {
	args: dropZoneArgs,
	play: async ({ canvasElement }) => {
		onUpload.mockClear();
		onUnsupportedFile.mockClear();
		const dropZone = getDropZone(canvasElement);
		const file = new File([""], "bad.txt");

		dispatchDrag(dropZone, "drop", { dataTransfer: fileTransfer(file) });

		await waitFor(() => expect(onUnsupportedFile).toHaveBeenCalledTimes(1));
		expect(onUnsupportedFile.mock.calls[0][0].name).toBe("bad.txt");
		expect(onUpload).not.toHaveBeenCalled();
		await expect(dropZone).toHaveAttribute("data-drag-active", "false");
	},
};

export const DropSupportedFile: Story = {
	args: dropZoneArgs,
	play: async ({ canvasElement }) => {
		onUpload.mockClear();
		onUnsupportedFile.mockClear();
		const dropZone = getDropZone(canvasElement);
		const file = new File([""], "template.zip");

		dispatchDrag(dropZone, "drop", { dataTransfer: fileTransfer(file) });

		await waitFor(() => expect(onUpload).toHaveBeenCalledTimes(1));
		expect(onUpload.mock.calls[0][0].name).toBe("template.zip");
		expect(onUnsupportedFile).not.toHaveBeenCalled();
	},
};
