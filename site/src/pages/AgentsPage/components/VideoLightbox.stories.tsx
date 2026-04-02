import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fireEvent, fn, waitFor, within } from "storybook/test";
import { RECORDING_UNAVAILABLE_TEXT } from "./ChatElements/tools/previewConstants";
import { VideoLightbox } from "./VideoLightbox";

// The file is stored in site/.storybook/static/tiny-recording.mp4.
const TINY_MP4 = "/tiny-recording.mp4";

const meta: Meta<typeof VideoLightbox> = {
	title: "components/VideoLightbox",
	component: VideoLightbox,
	decorators: [
		(Story) => (
			<div className="flex min-h-64 items-center justify-center p-8 text-content-primary">
				<p>Background content behind the lightbox overlay</p>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof VideoLightbox>;

export const Default: Story = {
	args: {
		src: TINY_MP4,
		open: true,
		onClose: fn(),
	},
	play: async ({ canvasElement }) => {
		const doc = canvasElement.ownerDocument;
		const video = doc.querySelector("video");
		expect(video).toBeInTheDocument();
		expect(video).toHaveAttribute("controls");
	},
};

export const AccessibleTitle: Story = {
	args: {
		src: TINY_MP4,
		open: true,
		onClose: fn(),
	},
	play: async ({ canvasElement }) => {
		const screen = within(canvasElement.ownerDocument.body);
		expect(
			screen.getByRole("dialog", { name: "Recording playback" }),
		).toBeInTheDocument();
	},
};

export const VideoError: Story = {
	args: {
		src: TINY_MP4,
		open: true,
		onClose: fn(),
	},
	play: async ({ canvasElement }) => {
		const doc = canvasElement.ownerDocument;
		const screen = within(doc.body);
		const video = doc.querySelector("video");
		expect(video).not.toBeNull();
		fireEvent.error(video!);
		await waitFor(() => {
			expect(screen.getByText(RECORDING_UNAVAILABLE_TEXT)).toBeInTheDocument();
		});
		// The video element should be replaced by the error message.
		expect(doc.querySelector("video")).toBeNull();
	},
};
