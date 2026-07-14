import type { Meta, StoryObj } from "@storybook/react-vite";
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
