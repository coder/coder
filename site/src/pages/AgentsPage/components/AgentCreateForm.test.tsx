import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import {
	mcpServerConfigsKey,
	organizationChatModelsKey,
	userChatPersonalModelOverrides,
} from "#/api/queries/chats";
import { preferenceSettingsKey } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	AgentCreateForm,
	type ParentChatTarget,
	selectedOrganizationIdStorageKey,
} from "./AgentCreateForm";

const dashboard = {
	organizations: [MockDefaultOrganization, MockOrganization2],
	showOrganizations: false,
};

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => dashboard,
}));

const personalModelOverrides: TypesGen.UserChatPersonalModelOverridesResponse =
	{
		enabled: false,
		root: {
			context: "root",
			mode: "chat_default",
			model_config_id: "",
			is_set: false,
		},
		general: {
			context: "general",
			mode: "deployment_default",
			model_config_id: "",
			is_set: false,
		},
		explore: {
			context: "explore",
			mode: "deployment_default",
			model_config_id: "",
			is_set: false,
		},
		deployment_defaults: {
			general: { context: "general", model_config_id: "" },
			explore: { context: "explore", model_config_id: "" },
		},
	};

const seedOrganization = (
	queryClient: ReturnType<typeof createTestQueryClient>,
	organizationId: string,
) => {
	const catalog: TypesGen.OrganizationChatModelsResponse = {
		models: [
			{
				...MockChatModel,
				id: `model-${organizationId}`,
				organization_id: organizationId,
				is_default: true,
			},
		],
		providers: [MockChatModelProviderDescriptor],
		unsupported_providers: [],
	};
	queryClient.setQueryData(organizationChatModelsKey(organizationId), catalog);
	queryClient.setQueryData(
		userChatPersonalModelOverrides(organizationId).queryKey,
		personalModelOverrides,
	);
	queryClient.setQueryData(mcpServerConfigsKey(organizationId), []);
};

const parentChat: ParentChatTarget = {
	parentChatId: "parent-chat-1",
	parentChatTitle: "Fix flaky login test",
	parentOrganizationId: MockDefaultOrganization.id,
};

const renderForm = (props: {
	parentChat?: ParentChatTarget;
	onClearParentChat?: () => void;
}) => {
	const onCreateChat = vi.fn().mockResolvedValue(undefined);
	const queryClient = createTestQueryClient();
	for (const organization of dashboard.organizations) {
		seedOrganization(queryClient, organization.id);
	}
	queryClient.setQueryData(preferenceSettingsKey, MockUserPreferenceSettings);
	render(
		<AppProviders queryClient={queryClient}>
			<AgentCreateForm
				onCreateChat={onCreateChat}
				isCreating={false}
				createError={undefined}
				canCreateChat
				canConfigureAgentSetup={false}
				workspaceCount={0}
				workspaceOptions={[]}
				workspacesError={undefined}
				isWorkspacesLoading={false}
				{...props}
			/>
		</AppProviders>,
	);
	return { onCreateChat };
};

const submitMessage = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("textbox", { name: "Chat message" }));
	await user.paste("Investigate the flaky test");
	const sendButton = screen.getByRole("button", { name: "Send" });
	await waitFor(() => expect(sendButton).toBeEnabled());
	await user.click(sendButton);
};

// jsdom's Range lacks getBoundingClientRect, which the Lexical editor
// reads while positioning; a spy needs an existing method to wrap.
beforeEach(() => {
	localStorage.clear();
	Range.prototype.getBoundingClientRect ??= () => new DOMRect();
	vi.spyOn(Range.prototype, "getBoundingClientRect").mockReturnValue(
		new DOMRect(0, 0, 1, 16),
	);
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("AgentCreateForm parent chat", () => {
	it("sends parentChatId and the parent's organization when it is permitted", async () => {
		const user = userEvent.setup();
		const { onCreateChat } = renderForm({
			parentChat: { ...parentChat, parentOrganizationId: MockOrganization2.id },
		});

		await submitMessage(user);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({
				parentChatId: parentChat.parentChatId,
				organizationId: MockOrganization2.id,
			}),
		);
	});

	it("keeps the stored organization preference when the parent pins another one", async () => {
		const user = userEvent.setup();
		const { onCreateChat } = renderForm({
			parentChat: { ...parentChat, parentOrganizationId: MockOrganization2.id },
		});

		await submitMessage(user);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
			MockDefaultOrganization.id,
		);
	});

	it("omits parentChatId when the parent's organization is not permitted", async () => {
		const user = userEvent.setup();
		const { onCreateChat } = renderForm({
			parentChat: { ...parentChat, parentOrganizationId: "org-elsewhere" },
		});

		await submitMessage(user);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		const options = onCreateChat.mock.calls[0][0];
		expect(options.parentChatId).toBeUndefined();
		expect(options.organizationId).toBe(MockDefaultOrganization.id);
	});

	it("clears the parent and moves focus to the message textbox on dismiss", async () => {
		const user = userEvent.setup();
		const onClearParentChat = vi.fn();
		renderForm({ parentChat, onClearParentChat });

		await user.click(
			screen.getByRole("button", {
				name: `Create under the root instead of ${parentChat.parentChatTitle}`,
			}),
		);

		expect(onClearParentChat).toHaveBeenCalledTimes(1);
		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).toHaveFocus(),
		);
	});
});
