import type { Meta, StoryObj } from "@storybook/react";
import {
	Tag as TagComponent,
	Tags as TagsComponent,
	TruncateTags as TruncateTagsComponent,
} from "./Tags";

const meta: Meta = {
	title: "pages/OrganizationProvisionerJobsPage/Tags",
};

export default meta;
type Story = StoryObj;

export const Tag: Story = {
	render: () => {
		return <TagComponent label="cluster" value="dogfood-v2" />;
	},
};

export const Tags: Story = {
	render: () => {
		return (
			<TagsComponent>
				<TagComponent label="cluster" value="dogfood-v2" />
				<TagComponent label="env" value="gke" />
				<TagComponent label="scope" value="organization" />
			</TagsComponent>
		);
	},
};

export const TruncateTags: Story = {
	render: () => {
		return (
			<TruncateTagsComponent
				tags={{
					cluster: "dogfood-v2",
					env: "gke",
					scope: "organization",
				}}
			/>
		);
	},
};
