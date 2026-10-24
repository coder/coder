import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { ChatProjectACL, ChatProjectUser } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectInstructions,
	MockEveryoneGroup,
	MockGroup,
	MockUnsetChatProjectInstructions,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ProjectDetailsPanelView } from "./ProjectDetailsPanelView";
import { buildAccessEntries } from "./projectAccess";

const mockSharees: ChatProjectUser[] = [
	"Ada Lovelace",
	"Grace Hopper",
	"Alan Turing",
	"Katherine Johnson",
	"Linus Torvalds",
	"Margaret Hamilton",
	"Dennis Ritchie",
	"Barbara Liskov",
	"Ken Thompson",
].map((name, index) => ({
	...MockUserMember,
	id: `sharee-${index}`,
	username: name.toLowerCase().replace(" ", "."),
	name,
	role: index === 0 ? "admin" : "use",
}));

const mockACL: ChatProjectACL = {
	owner: MockUserOwner,
	users: mockSharees.slice(0, 2),
	groups: [],
};

const mockMembers = {
	entries: buildAccessEntries(mockACL),
	error: undefined,
	isRetrying: false,
	onRetry: fn(),
	onOpenAccess: fn(),
};

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
		members: mockMembers,
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

/** The ACL names the owner, so both load together. */
export const OwnerLoading: Story = {
	args: {
		ownerLabel: undefined,
		members: { ...mockMembers, entries: undefined },
	},
};

export const OtherOwner: Story = {
	args: {
		ownerLabel: "Jane Doe",
		members: {
			...mockMembers,
			entries: buildAccessEntries({
				...mockACL,
				owner: { ...MockUserMember, name: "Jane Doe" },
			}),
		},
	},
};

/** The ACL failed to load, so it could not name the owner. */
export const OwnerUnknown: Story = {
	args: {
		ownerLabel: "Unknown",
		members: {
			...mockMembers,
			entries: undefined,
			error: mockApiError({ message: "Failed to get chat project ACL." }),
		},
	},
};

/** Long owner and organization names wrap so they stay readable. */
export const LongNames: Story = {
	args: {
		ownerLabel: "bartholomewmontgomeryfitzgerald1",
		organizationLabel: "Platform-Infrastructure-Engineering-Europe",
		members: {
			...mockMembers,
			entries: buildAccessEntries({
				...mockACL,
				owner: {
					...MockUserMember,
					username: "bartholomewmontgomeryfitzgerald1",
					name: "",
				},
			}),
		},
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

/** Chat sharing is disabled for the deployment, so the members are hidden. */
export const SharingDisabled: Story = {
	args: { members: undefined },
};

export const OwnerOnly: Story = {
	args: {
		members: {
			...mockMembers,
			entries: buildAccessEntries({
				...mockACL,
				users: [],
				groups: [],
			}),
		},
	},
};

/** Entries past the visible limit collapse into a "+K" tile. */
export const MembersOverflow: Story = {
	args: {
		members: {
			...mockMembers,
			entries: buildAccessEntries({
				...mockACL,
				users: mockSharees,
				groups: [],
			}),
		},
	},
};

/** Keyboard focus on the "+K" tile lists the hidden members below the row. */
export const MembersOverflowFocused: Story = {
	args: MembersOverflow.args,
	play: async ({ canvasElement }) => {
		within(canvasElement).getByRole("button", { name: "3 more" }).focus();
	},
};

/** A full row shows every avatar instead of a "+1" tile. */
export const MembersFullRow: Story = {
	args: {
		members: {
			...mockMembers,
			entries: buildAccessEntries({
				...mockACL,
				users: mockSharees.slice(0, 7),
				groups: [],
			}),
		},
	},
};

/** Groups follow users and count separately in the heading. */
export const MembersWithGroups: Story = {
	args: {
		members: {
			...mockMembers,
			entries: buildAccessEntries({
				...mockACL,
				users: mockSharees.slice(0, 2),
				groups: [
					{ ...MockGroup, role: "use" },
					{ ...MockEveryoneGroup, role: "use" },
				],
			}),
		},
	},
};

/** Hovering a member avatar shows the member's name. */
export const MemberTooltip: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.hover(within(canvasElement).getByText("AD"));
	},
};

/** Long names wrap instead of running off narrow screens. */
export const MemberTooltipLongName: Story = {
	args: {
		members: {
			...mockMembers,
			entries: buildAccessEntries({
				...mockACL,
				users: [
					{
						...mockSharees[0],
						name: "Augusta Ada King, Countess of Lovelace, Analyst of the Analytical Engine and Poet of Science",
					},
				],
				groups: [],
			}),
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.hover(within(canvasElement).getByText("AU"));
	},
};

export const MembersLoading: Story = {
	args: { members: { ...mockMembers, entries: undefined } },
};

export const MembersLoadError: Story = {
	args: {
		members: {
			...mockMembers,
			entries: undefined,
			error: mockApiError({ message: "Failed to get chat project ACL." }),
		},
	},
};

/** A failed refetch keeps the cached members and shows the error. */
export const MembersRefetchError: Story = {
	args: {
		members: {
			...mockMembers,
			error: mockApiError({ message: "Failed to get chat project ACL." }),
		},
	},
};
