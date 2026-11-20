import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import type { ChatProject, ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockArchivedChatProject,
	MockChatProject,
	MockChatProjectSummary,
	MockUserMember,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectsPageView } from "./ProjectsPageView";

const projectSummary = (
	project: Pick<ChatProject, "id" | "name"> & Partial<ChatProject>,
	overrides: Partial<ChatProjectSummary> = {},
): ChatProjectSummary => ({
	...MockChatProjectSummary,
	pinned: false,
	pin_order: 0,
	...overrides,
	project: { ...MockChatProject, description: "", ...project },
});

const mockYourProjects: ChatProjectSummary[] = [
	projectSummary(
		{ id: "launch", name: "Launch", description: "Chats for the launch work." },
		{
			pinned: true,
			pin_order: 1,
			own_chat_count: 4,
			own_needs_attention: true,
			own_running: true,
			member_count: 4,
			last_activity_at: "2026-11-10T12:00:00Z",
		},
	),
	projectSummary(
		{
			id: "docs",
			name: "Docs site",
			description: "Rewrites and fixes for the public documentation.",
		},
		{
			pinned: true,
			pin_order: 2,
			own_chat_count: 2,
			own_running: true,
			last_activity_at: "2026-11-09T12:00:00Z",
		},
	),
	projectSummary(
		{
			id: "billing",
			name: "Billing",
			description:
				"Invoices, usage metering, refunds, and the payment provider integration, including the webhooks that reconcile failed charges every night.",
		},
		{
			own_chat_count: 3,
			own_unread_count: 1,
			member_count: 1234,
			last_activity_at: "2026-11-08T12:00:00Z",
		},
	),
	projectSummary(
		{
			id: "design",
			name: "Design reviews",
			description: "Feedback rounds on new screens.",
			icon: "/emojis/1f3a8.png",
		},
		{
			own_chat_count: 3,
			member_count: 2,
			last_activity_at: "2026-10-01T12:00:00Z",
		},
	),
	projectSummary({ id: "empty", name: "Experiments" }),
];

const mockSharedProjects: ChatProjectSummary[] = [
	projectSummary(
		{
			id: "oncall",
			name: "On-call",
			description: "Incident follow-ups for the platform team.",
			owner_id: MockUserMember.id,
			permissions: {
				update: false,
				delete: false,
				share: false,
				archive: false,
			},
		},
		{
			own_chat_count: 1,
			member_count: 12,
			last_activity_at: "2026-11-10T08:00:00Z",
		},
	),
];

const meta: Meta<typeof ProjectsPageView> = {
	title: "pages/AgentsPage/ProjectsPage/ProjectsPageView",
	component: ProjectsPageView,
	decorators: [
		(Story) => (
			<div className="flex h-[720px] flex-col">
				<Story />
			</div>
		),
	],
	args: {
		tab: "yours",
		onTabChange: fn(),
		projects: { yours: mockYourProjects, shared: mockSharedProjects },
		onRetry: fn(),
		projectLinkSearch: "",
		onCreateProject: fn(),
		onPin: fn(),
		onUnpin: fn(),
		pendingPinIds: new Set(),
		onEdit: fn(),
		onArchive: fn(),
		onDelete: fn(),
		showArchivedTab: false,
		archivedProjects: [],
		archivedError: undefined,
		onRetryArchived: fn(),
		onUnarchived: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectsPageView>;

export const YourProjects: Story = {};

export const SharedWithYou: Story = { args: { tab: "shared" } };

export const Loading: Story = { args: { projects: undefined } };

const mockArchivedProjects: ChatProject[] = [
	{
		...MockArchivedChatProject,
		id: "archived-release",
		name: "Release 2.0",
		description: "Chats from the 2.0 release.",
	},
	{
		...MockArchivedChatProject,
		id: "archived-shared",
		name: "Shared archive",
		description: "",
		owner_id: MockUserMember.id,
		permissions: { update: false, delete: false, share: false, archive: false },
	},
];

export const Archived: Story = {
	args: {
		tab: "archived",
		showArchivedTab: true,
		archivedProjects: mockArchivedProjects,
	},
};

export const ArchivedRefetchError: Story = {
	args: {
		...Archived.args,
		archivedError: mockApiError({
			message: "Failed to refresh archived projects.",
		}),
	},
};

export const YourProjectsWithArchived: Story = {
	args: { showArchivedTab: true, archivedProjects: mockArchivedProjects },
};

export const ArchivedActionsMenuOpen: Story = {
	args: Archived.args,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Open project actions for Release 2.0",
			}),
		);
	},
};

export const ArchivedLoading: Story = {
	args: { ...Archived.args, archivedProjects: undefined },
};

export const ArchivedLoadError: Story = {
	args: {
		...Archived.args,
		archivedProjects: undefined,
		archivedError: mockApiError({
			message: "Failed to load archived projects.",
		}),
	},
};

export const LoadError: Story = {
	args: {
		projects: undefined,
		error: mockApiError({ message: "Failed to load projects." }),
	},
};

export const NoProjects: Story = {
	args: { projects: { yours: [], shared: mockSharedProjects } },
};

export const NoSharedProjects: Story = {
	args: { tab: "shared", projects: { yours: mockYourProjects, shared: [] } },
};

/** Focusing the pin button swaps its icon to unpin and opens its tooltip. */
export const UnpinFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement).getByRole("button", { name: "Unpin Launch" }).focus();
		await screen.findByRole("tooltip");
	},
};

export const ActionsMenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Open project actions for Experiments",
			}),
		);
	},
};

export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
};
