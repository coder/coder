import type { Meta, StoryObj } from "@storybook/react";
import {
	ProvisionerTag,
	ProvisionerTags,
	ProvisionerTruncateTags,
} from "./ProvisionerTags";

const meta: Meta = {
	title: "modules/provisioners/ProvisionerTags",
};

export default meta;
type Story = StoryObj;

export const Tag: Story = {
	render: () => {
		return <ProvisionerTag label="cluster" value="dogfood-v2" />;
	},
};

export const Tags: Story = {
	render: () => {
		return (
			<ProvisionerTags>
				<ProvisionerTag label="cluster" value="dogfood-v2" />
				<ProvisionerTag label="env" value="gke" />
				<ProvisionerTag label="scope" value="organization" />
			</ProvisionerTags>
		);
	},
};

export const TruncateTags: Story = {
	render: () => {
		return (
			<ProvisionerTruncateTags
				tags={{
					cluster: "dogfood-v2",
					env: "gke",
					scope: "organization",
				}}
			/>
		);
	},
};
