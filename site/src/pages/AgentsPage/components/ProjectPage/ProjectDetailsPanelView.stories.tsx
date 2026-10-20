import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import {
	MockChatProject,
	MockChatProjectInstructions,
	MockUnsetChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ProjectDetailsPanelView } from "./ProjectDetailsPanelView";

const meta: Meta<typeof ProjectDetailsPanelView> = {
	title: "pages/AgentsPage/ProjectPage/ProjectDetailsPanelView",
	component: ProjectDetailsPanelView,
	decorators: [
		(Story) => (
			<div className="max-w-[310px]">
				<Story />
			</div>
		),
	],
	args: {
		ownerLabel: "You",
		createdAt: MockChatProject.created_at,
		skillCount: 3,
		disabledSkillCount: 0,
		skillsError: undefined,
		onOpenSkills: fn(),
		instructions: MockChatProjectInstructions,
		error: undefined,
		onRetry: fn(),
		onEditInstructions: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectDetailsPanelView>;

export const Default: Story = {};

/** Users without update permission, such as `use` sharees, cannot edit. */
export const WithoutUpdatePermission: Story = {
	args: { onEditInstructions: undefined },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(/Run the linters before committing/);
		expect(
			canvas.queryByRole("button", { name: "Edit instructions" }),
		).not.toBeInTheDocument();
	},
};

export const UnsetWithoutUpdatePermission: Story = {
	args: {
		instructions: MockUnsetChatProjectInstructions,
		onEditInstructions: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("No instructions");
		expect(
			canvas.queryByRole("button", { name: "Create instructions" }),
		).not.toBeInTheDocument();
	},
};

export const WithOrganization: Story = {
	args: { organizationLabel: "Coder" },
};

export const OwnerLoading: Story = {
	args: { ownerLabel: undefined },
};

export const OtherOwner: Story = {
	args: { ownerLabel: "Jane Doe" },
};

export const OwnerUnknown: Story = {
	args: { ownerLabel: "Unknown" },
};

/** Long owner and organization names wrap so they stay readable. */
export const LongNames: Story = {
	args: {
		ownerLabel: "bartholomewmontgomeryfitzgerald1",
		organizationLabel: "Platform-Infrastructure-Engineering-Europe",
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

/** Focus returns to the Skills row after the skills dialog closes. */
export const SkillsFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: /^Skills:/ })
			.focus();
	},
};

export const OneSkill: Story = {
	args: { skillCount: 1 },
};

export const SomeSkillsDisabled: Story = {
	args: { disabledSkillCount: 1 },
};

export const NoSkills: Story = {
	args: { skillCount: 0 },
};

export const Unset: Story = {
	args: { instructions: MockUnsetChatProjectInstructions },
};

/** Long instructions are clamped to three lines in the preview. */
export const LongInstructions: Story = {
	args: {
		instructions: {
			...MockChatProjectInstructions,
			instructions: Array.from(
				{ length: 8 },
				(_, index) =>
					`${index + 1}. Follow the repository conventions for this kind of change and explain the tradeoffs in the pull request description.`,
			).join("\n"),
		},
	},
};

export const Loading: Story = {
	args: { instructions: undefined, skillCount: undefined },
};

export const LoadError: Story = {
	args: {
		instructions: undefined,
		skillCount: undefined,
		skillsError: mockApiError({ message: "Failed to list project skills." }),
		error: mockApiError({
			message: "Failed to get chat project instructions.",
		}),
	},
};

/** A failed refetch keeps the cached instructions and shows the error. */
export const RefetchError: Story = {
	args: {
		error: mockApiError({
			message: "Failed to get chat project instructions.",
		}),
	},
};
