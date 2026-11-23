import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FormikErrors, type FormikTouched, useFormik } from "formik";
import { fn, screen, userEvent, within } from "storybook/test";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockPersonalModelOptions,
	MockPersonalReasoningChatModel,
} from "#/testHelpers/chatModels";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	AgentCreateAutomationForm,
	type CreateAutomationFormValues,
} from "./AgentCreateAutomationForm";

type AutomationFormStoryProps = Omit<
	React.ComponentProps<typeof AgentCreateAutomationForm>,
	"form"
> & {
	initialValues?: Partial<CreateAutomationFormValues>;
	initialErrors?: FormikErrors<CreateAutomationFormValues>;
	initialTouched?: FormikTouched<CreateAutomationFormValues>;
};

const mockOwnedChat: Chat = {
	...MockChat,
	organization_id: MockDefaultOrganization.id,
	title: "Review launch checklist",
};

const mockLongChat: Chat = {
	...mockOwnedChat,
	id: "chat-with-long-title",
	title:
		"Investigate the multi-region deployment pipeline and summarize all remaining launch risks for the infrastructure team",
};

const AutomationFormStory: React.FC<AutomationFormStoryProps> = ({
	initialValues,
	initialErrors,
	initialTouched,
	...props
}) => {
	const form = useFormik<CreateAutomationFormValues>({
		initialValues: {
			organization_id: MockDefaultOrganization.id,
			name: "Daily review",
			prompt:
				"Summarize the open pull requests and highlight anything blocked.",
			kind: "schedule",
			target_mode: "new_chat",
			enabled: true,
			schedule_cron: "0 9 * * 1-5",
			schedule_time_zone: "UTC",
			when_busy: "skip",
			webhook_auth: "bearer",
			webhook_secret: "",
			target_chat_id: "",
			new_chat_model_config_id: MockPersonalReasoningChatModel.id,
			project_id: MockChatProject.id,
			reasoning_effort: "medium",
			...initialValues,
		},
		initialErrors,
		initialTouched,
		onSubmit: fn(),
	});

	return <AgentCreateAutomationForm {...props} form={form} />;
};

const meta = {
	title: "pages/AgentsPage/AgentCreateAutomationForm",
	component: AutomationFormStory,
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		modelOptions: MockPersonalModelOptions,
		projects: [MockChatProject],
		chats: [mockOwnedChat, mockLongChat],
		loadingModels: false,
		loadingProjects: false,
		loadingChats: false,
		hasMoreChats: false,
		loadingMoreChats: false,
		onLoadMoreChats: fn(),
		isPending: false,
		submitError: undefined,
		onCancel: fn(),
	},
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl p-4 pt-8">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof AutomationFormStory>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ScheduleNewChat: Story = {};

export const ExistingChatBusy: Story = {
	args: {
		initialValues: {
			target_mode: "existing_chat",
			target_chat_id: mockOwnedChat.id,
			when_busy: "queue",
		},
		hasMoreChats: true,
	},
};

export const WebhookBearer: Story = {
	args: { initialValues: { kind: "webhook" } },
};

export const WebhookGitHub: Story = {
	args: { initialValues: { kind: "webhook", webhook_auth: "github" } },
};

export const WebhookStandardWebhooks: Story = {
	args: {
		initialValues: {
			kind: "webhook",
			webhook_auth: "standard_webhooks",
			webhook_secret: "",
		},
	},
};

export const ModelPickerOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Model" }),
		);
	},
};

export const TimeZonePickerOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Time zone" }),
		);
		await userEvent.type(
			screen.getByRole("combobox", { name: "Search time zone" }),
			"America/New_York",
		);
	},
};

export const ChatPickerOpen: Story = {
	args: ExistingChatBusy.args,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Existing chat" }),
		);
	},
};

export const LoadingModels: Story = {
	args: {
		modelOptions: [],
		loadingModels: true,
		initialValues: { new_chat_model_config_id: "" },
	},
};

export const EmptyResources: Story = {
	args: {
		modelOptions: [],
		projects: [],
		initialValues: { new_chat_model_config_id: "", project_id: "" },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Model" }),
		);
	},
};

export const EmptyProjects: Story = {
	args: {
		projects: [],
		initialValues: { project_id: "" },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Project" }),
		);
	},
};

export const EmptyChats: Story = {
	args: {
		chats: [],
		initialValues: { target_mode: "existing_chat", target_chat_id: "" },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Existing chat" }),
		);
	},
};

export const SubmitError: Story = {
	args: {
		submitError: {
			message: "Could not create the automation.",
			detail: "Check the automations list before trying again.",
		},
	},
};

export const FieldErrors: Story = {
	args: {
		initialValues: {
			name: "",
			prompt: "",
			new_chat_model_config_id: "",
			schedule_cron: "invalid",
		},
		initialErrors: {
			name: "Enter a name.",
			prompt: "Enter a prompt.",
			new_chat_model_config_id: "Select a model.",
			schedule_cron: "Enter a five-field cron expression.",
		},
		initialTouched: {
			name: true,
			prompt: true,
			new_chat_model_config_id: true,
			schedule_cron: true,
		},
	},
};

export const Pending: Story = {
	args: { isPending: true },
};

export const LongLabels: Story = {
	args: {
		modelOptions: [
			{
				...MockPersonalModelOptions[2],
				displayName:
					"Advanced reasoning model for an unusually long release engineering project title",
			},
		],
		projects: [
			{
				...MockChatProject,
				name: "Cross-functional launch planning and infrastructure readiness for all regions",
			},
		],
	},
};

export const Mobile: Story = {
	...ScheduleNewChat,
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
