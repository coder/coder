import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import type { UserChatPersonalModelOverridesResponse } from "#/api/typesGenerated";
import {
	MockPersonalClaudeChatModel,
	MockPersonalDefaultChatModel,
	MockPersonalModelOptions,
	MockPersonalReasoningChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import type { AgentSettingsUserAgentsPageViewProps } from "./AgentSettingsUserAgentsPageView";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";

const defaultProps: AgentSettingsUserAgentsPageViewProps = {
	overridesData: {
		...MockUnsetUserChatPersonalModelOverrides,
		deployment_defaults: {
			general: {
				context: "general",
				model_config_id: MockPersonalClaudeChatModel.id,
			},
			explore: {
				context: "explore",
				model_config_id: MockPersonalClaudeChatModel.id,
			},
		},
	},
	overridesError: undefined,
	onRetryOverrides: vi.fn(),
	isRetryingOverrides: false,
	isLoading: false,
	modelOptions: MockPersonalModelOptions,
	models: [
		MockPersonalDefaultChatModel,
		MockPersonalClaudeChatModel,
		MockPersonalReasoningChatModel,
	],
	modelsError: undefined,
	organizations: [MockDefaultOrganization],
	selectedOrganization: MockDefaultOrganization,
	onSelectOrganization: vi.fn(),
	onSaveOverride: vi.fn(),
	isSaving: false,
};

const rootModelOverride = (
	modelId: string,
): UserChatPersonalModelOverridesResponse => ({
	...MockUnsetUserChatPersonalModelOverrides,
	root: {
		...MockUnsetUserChatPersonalModelOverrides.root,
		mode: "model",
		model_config_id: modelId,
		is_set: true,
	},
});

const renderWithOverrides = (
	onSaveOverride: AgentSettingsUserAgentsPageViewProps["onSaveOverride"],
) => {
	let setOverrides: React.Dispatch<
		React.SetStateAction<UserChatPersonalModelOverridesResponse>
	> = () => {};
	const View = () => {
		const [overrides, setData] =
			useState<UserChatPersonalModelOverridesResponse>(
				MockUnsetUserChatPersonalModelOverrides,
			);
		setOverrides = setData;
		return (
			<AgentSettingsUserAgentsPageView
				{...defaultProps}
				onSaveOverride={onSaveOverride}
				overridesData={overrides}
			/>
		);
	};
	render(<View />);
	return (overrides: UserChatPersonalModelOverridesResponse) => {
		act(() => setOverrides(overrides));
	};
};

const selectModel = async (
	user: ReturnType<typeof userEvent.setup>,
	sectionName: string,
	optionName: RegExp,
) => {
	const section = screen.getByRole("region", { name: sectionName });
	await user.click(within(section).getByRole("combobox"));
	await user.click(await screen.findByRole("option", { name: optionName }));
	return section;
};

describe("AgentSettingsUserAgentsPageView", () => {
	it("saves a root model and a general chat default with the selected organization", async () => {
		const user = userEvent.setup();
		const onSaveOverride = vi.fn();
		render(
			<AgentSettingsUserAgentsPageView
				{...defaultProps}
				onSaveOverride={onSaveOverride}
			/>,
		);
		const root = await selectModel(
			user,
			"Root agent model",
			/Claude Sonnet 4/i,
		);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: { mode: "model", model_config_id: MockPersonalClaudeChatModel.id },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);

		const general = await selectModel(
			user,
			"General subagent model",
			/Chat default/i,
		);
		await user.click(within(general).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "general",
				req: { mode: "chat_default", model_config_id: "" },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});

	it("saves a reasoning model with the chosen effort", async () => {
		const user = userEvent.setup();
		const onSaveOverride = vi.fn();
		render(
			<AgentSettingsUserAgentsPageView
				{...defaultProps}
				onSaveOverride={onSaveOverride}
				overridesData={{
					...MockUnsetUserChatPersonalModelOverrides,
					root: {
						...MockUnsetUserChatPersonalModelOverrides.root,
						mode: "model",
						model_config_id: MockPersonalDefaultChatModel.id,
						is_set: true,
					},
				}}
			/>,
		);
		const root = await selectModel(user, "Root agent model", /GPT-5/i);
		await user.tab();
		await user.tab();
		await user.keyboard("{ArrowRight}");
		await user.keyboard("{Escape}");
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: {
					mode: "model",
					model_config_id: MockPersonalReasoningChatModel.id,
					reasoning_effort: "high",
				},
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});

	it("retries a failed overrides request", async () => {
		const user = userEvent.setup();
		const onRetryOverrides = vi.fn();
		render(
			<AgentSettingsUserAgentsPageView
				{...defaultProps}
				overridesData={undefined}
				overridesError={new Error("Failed to load overrides")}
				onRetryOverrides={onRetryOverrides}
			/>,
		);
		await user.click(screen.getByRole("button", { name: "Retry" }));
		expect(onRetryOverrides).toHaveBeenCalledOnce();
	});

	it("repairs an invalid root organization default", async () => {
		const user = userEvent.setup();
		const onSaveOverride = vi.fn();
		render(
			<AgentSettingsUserAgentsPageView
				{...defaultProps}
				onSaveOverride={onSaveOverride}
				overridesData={{
					...MockUnsetUserChatPersonalModelOverrides,
					root: {
						...MockUnsetUserChatPersonalModelOverrides.root,
						mode: "deployment_default",
						is_set: true,
					},
				}}
			/>,
		);
		const root = await selectModel(user, "Root agent model", /Chat default/i);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: { mode: "chat_default", model_config_id: "" },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});

	it("recovers an unavailable saved model when the catalog is empty", async () => {
		const user = userEvent.setup();
		const onSaveOverride = vi.fn();
		render(
			<AgentSettingsUserAgentsPageView
				{...defaultProps}
				modelOptions={[]}
				models={[]}
				onSaveOverride={onSaveOverride}
				overridesData={{
					...MockUnsetUserChatPersonalModelOverrides,
					root: {
						...MockUnsetUserChatPersonalModelOverrides.root,
						mode: "model",
						model_config_id: "model-stale",
						is_set: true,
					},
				}}
			/>,
		);
		const root = await selectModel(user, "Root agent model", /Chat default/i);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: { mode: "chat_default", model_config_id: "" },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});

	it("preserves a dirty selection across server refetches", async () => {
		const user = userEvent.setup();
		const onSaveOverride =
			vi.fn<AgentSettingsUserAgentsPageViewProps["onSaveOverride"]>();
		const refetch = renderWithOverrides(onSaveOverride);
		const root = await selectModel(
			user,
			"Root agent model",
			/Claude Sonnet 4/i,
		);
		refetch(rootModelOverride(MockPersonalDefaultChatModel.id));
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: { mode: "model", model_config_id: MockPersonalClaudeChatModel.id },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});

	it("accepts a server refetch when the form is pristine", async () => {
		const user = userEvent.setup();
		const onSaveOverride =
			vi.fn<AgentSettingsUserAgentsPageViewProps["onSaveOverride"]>();
		const refetch = renderWithOverrides(onSaveOverride);
		refetch(rootModelOverride(MockPersonalClaudeChatModel.id));
		const root = await selectModel(user, "Root agent model", /Chat default/i);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: { mode: "chat_default", model_config_id: "" },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});

	it("clears the draft after success and accepts a later refetch", async () => {
		const user = userEvent.setup();
		const onSaveOverride =
			vi.fn<AgentSettingsUserAgentsPageViewProps["onSaveOverride"]>();
		const refetch = renderWithOverrides(onSaveOverride);
		const root = await selectModel(
			user,
			"Root agent model",
			/Claude Sonnet 4/i,
		);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		const saveCallbacks = onSaveOverride.mock.calls[0]?.[1];
		expect(saveCallbacks?.onSuccess).toEqual(expect.any(Function));
		refetch(rootModelOverride(MockPersonalClaudeChatModel.id));
		act(() => saveCallbacks?.onSuccess?.());
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledTimes(1);

		refetch(rootModelOverride(MockPersonalDefaultChatModel.id));
		await selectModel(user, "Root agent model", /Chat default/i);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenLastCalledWith(
			{
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: { mode: "chat_default", model_config_id: "" },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
		expect(onSaveOverride).toHaveBeenCalledTimes(2);
	});

	it("keeps an edited payload available for retry after failure", async () => {
		const user = userEvent.setup();
		const onSaveOverride =
			vi.fn<AgentSettingsUserAgentsPageViewProps["onSaveOverride"]>();
		const refetch = renderWithOverrides(onSaveOverride);
		const root = await selectModel(
			user,
			"Root agent model",
			/Claude Sonnet 4/i,
		);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		act(() => {
			onSaveOverride.mock.calls[0]?.[1]?.onError?.();
		});
		refetch(rootModelOverride(MockPersonalDefaultChatModel.id));
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledTimes(2);
		for (const [payload] of onSaveOverride.mock.calls) {
			expect(payload).toEqual({
				organizationId: MockDefaultOrganization.id,
				context: "root",
				req: { mode: "model", model_config_id: MockPersonalClaudeChatModel.id },
			});
		}
	});

	it("uses the switched organization for subsequent saves", async () => {
		const user = userEvent.setup();
		const onSaveOverride = vi.fn();
		const onSelectOrganization = vi.fn();
		const OrganizationView = () => {
			const [selectedOrganization, setSelectedOrganization] = useState(
				MockDefaultOrganization,
			);
			return (
				<AgentSettingsUserAgentsPageView
					{...defaultProps}
					onSaveOverride={onSaveOverride}
					organizations={[MockDefaultOrganization, MockOrganization2]}
					selectedOrganization={selectedOrganization}
					onSelectOrganization={(organization) => {
						onSelectOrganization(organization);
						setSelectedOrganization(organization);
					}}
				/>
			);
		};
		render(<OrganizationView />);
		await user.click(
			screen.getByRole("button", {
				name: new RegExp(MockDefaultOrganization.display_name, "i"),
			}),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(MockOrganization2.display_name, "i"),
			}),
		);
		expect(onSelectOrganization).toHaveBeenCalledWith(MockOrganization2);
		const root = await selectModel(
			user,
			"Root agent model",
			/Claude Sonnet 4/i,
		);
		await user.click(within(root).getByRole("button", { name: "Save" }));
		expect(onSaveOverride).toHaveBeenCalledWith(
			{
				organizationId: MockOrganization2.id,
				context: "root",
				req: { mode: "model", model_config_id: MockPersonalClaudeChatModel.id },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});
});
