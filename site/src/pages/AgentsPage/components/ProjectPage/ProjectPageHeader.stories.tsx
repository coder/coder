import type { Meta, StoryObj } from "@storybook/react-vite";
import { EllipsisVerticalIcon } from "lucide-react";
import { within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { MockChatProject } from "#/testHelpers/entities";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ProjectMetadataBadges } from "./ProjectMetadataBadges";
import { ProjectPageHeader } from "./ProjectPageHeader";

const actions = (
	<Button variant="subtle" size="icon" aria-label="Project actions">
		<EllipsisVerticalIcon />
	</Button>
);

const meta: Meta<typeof ProjectPageHeader> = {
	title: "pages/AgentsPage/ProjectPage/ProjectPageHeader",
	component: ProjectPageHeader,
	decorators: [
		(Story) => (
			<div className="max-w-5xl p-6">
				<Story />
			</div>
		),
	],
	args: {
		project: MockChatProject,
		actions,
		metadata: (
			<ProjectMetadataBadges
				ownerLabel="you"
				createdAt={MockChatProject.created_at}
			/>
		),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectPageHeader>;

export const Default: Story = {};

export const WithoutDescription: Story = {
	args: {
		project: { ...MockChatProject, description: "" },
	},
};

export const LongNameAndDescription: Story = {
	args: {
		project: {
			...MockChatProject,
			name: "N".repeat(64),
			description: "d".repeat(1024),
		},
	},
};

export const OwnerLoading: Story = {
	args: {
		metadata: (
			<ProjectMetadataBadges
				ownerLabel={undefined}
				createdAt={MockChatProject.created_at}
			/>
		),
	},
};

export const OtherOwner: Story = {
	args: {
		metadata: (
			<ProjectMetadataBadges
				ownerLabel="Jane Doe"
				createdAt={MockChatProject.created_at}
			/>
		),
	},
};

export const OwnerUnknown: Story = {
	args: {
		metadata: (
			<ProjectMetadataBadges
				ownerLabel="Unknown"
				createdAt={MockChatProject.created_at}
			/>
		),
	},
};

export const WithOrganization: Story = {
	args: {
		metadata: (
			<ProjectMetadataBadges
				ownerLabel="you"
				createdAt={MockChatProject.created_at}
				organizationLabel="Coder"
			/>
		),
	},
};

/** Keyboard focus on the creation date shows the full timestamp. */
export const CreatedDateFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", {
				name: formatDateTime(
					MockChatProject.created_at,
					DATE_FORMAT.MEDIUM_DATE,
				),
			})
			.focus();
	},
};
