import type { Meta, StoryObj } from "@storybook/react-vite";
import { fireEvent, fn } from "storybook/test";
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
};

export const VideoError: Story = {
	args: {
		src: TINY_MP4,
		open: true,
		onClose: fn(),
	},
	play: async ({ canvasElement }) => {
		const doc = canvasElement.ownerDocument;
		const video = doc.querySelector("video");
		fireEvent.error(video!);
	},
};
