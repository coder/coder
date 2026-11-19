import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import type { ChatAutomation } from "#/api/typesGenerated";
import {
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import {
	MockDefaultOrganization,
	MockOrganization2,
	mockApiError,
} from "#/testHelpers/entities";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";

const MockSecondOrganizationWebhookAutomation: ChatAutomation = {
	...MockWebhookChatAutomation,
	organization_id: MockOrganization2.id,
};

const MockUnavailableTargetAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "unavailable-target",
	name: "Weekly dependency review",
	prompt: "Review dependency updates and summarize security fixes.",
	organization_id: MockOrganization2.id,
	schedule_cron: "0 9 * * 1",
	schedule_time_zone: "America/New_York",
	owner: null,
	target_chat: null,
	paused_reasons: ["target_unavailable"],
};

const meta = {
	title: "pages/AgentsPage/AgentAutomationsPageView",
	component: AgentAutomationsPageView,
	args: {
		automations: [MockChatAutomation, MockWebhookChatAutomation],
		organizations: [MockDefaultOrganization],
		isLoading: false,
		loadErrors: [],
		onRetry: fn(),
		onMenuTrigger: fn(),
		onDeleteAutomation: fn(),
	},
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl p-4 pt-8">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof AgentAutomationsPageView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const ActionsMenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "More actions for Daily PR review",
			}),
		);
	},
};

export const FilterMenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", {
				name: "Search and filter automations…",
			}),
		);
	},
};

export const Filtered: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.type(
			within(canvasElement).getByRole("combobox", {
				name: "Search and filter automations…",
			}),
			"release{Enter}",
		);
	},
};

export const NoMatchingAutomations: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.type(
			within(canvasElement).getByRole("combobox", {
				name: "Search and filter automations…",
			}),
			"no matching automation{Enter}",
		);
	},
};

export const FiltersApplied: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", {
				name: "Search and filter automations…",
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Disabled" }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", {
				name: "Search and filter automations…",
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Webhook" }),
		);
	},
};

export const Empty: Story = {
	args: { automations: [] },
};

export const Loading: Story = {
	args: { automations: [], isLoading: true },
};

export const LoadError: Story = {
	args: {
		automations: [],
		loadErrors: [
			{
				organization: MockDefaultOrganization,
				error: mockApiError({ message: "Could not load automations." }),
				isRetrying: false,
			},
		],
	},
};

export const PartialLoadError: Story = {
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		loadErrors: [
			{
				organization: MockOrganization2,
				error: mockApiError({ message: "Could not load automations." }),
				isRetrying: false,
			},
		],
	},
};

export const PartialLoadErrorNoMatches: Story = {
	...PartialLoadError,
	play: NoMatchingAutomations.play,
};

export const LoadingAnotherOrganization: Story = {
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		isLoading: true,
	},
};

export const RefetchError: Story = {
	...FiltersApplied,
	args: {
		loadErrors: [
			{
				organization: MockDefaultOrganization,
				error: mockApiError({ message: "Could not refresh automations." }),
				isRetrying: true,
			},
		],
	},
};

export const MultipleOrganizations: Story = {
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		automations: [
			MockChatAutomation,
			MockSecondOrganizationWebhookAutomation,
			MockUnavailableTargetAutomation,
		],
	},
};

export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};

export const EmptyMobile: Story = {
	...Empty,
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
