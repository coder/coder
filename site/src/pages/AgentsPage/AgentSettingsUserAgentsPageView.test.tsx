import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";
import {
	buildArgs,
	buildOverride,
	buildOverridesResponse,
	claudeModelConfig,
	defaultModelConfig,
	reasoningModelConfig,
} from "./AgentSettingsUserAgentsPageView.stories";

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
			<AgentSettingsUserAgentsPageView {...buildArgs({ onSaveOverride })} />,
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
				req: { mode: "model", model_config_id: claudeModelConfig.id },
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
				{...buildArgs({
					onSaveOverride,
					overridesData: buildOverridesResponse({
						root: buildOverride("root", {
							mode: "model",
							model_config_id: defaultModelConfig.id,
							is_set: true,
						}),
					}),
				})}
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
					model_config_id: reasoningModelConfig.id,
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
				{...buildArgs({
					overridesData: undefined,
					overridesError: new Error("Failed to load overrides"),
					onRetryOverrides,
				})}
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
				{...buildArgs({
					onSaveOverride,
					overridesData: buildOverridesResponse({
						root: buildOverride("root", {
							mode: "deployment_default",
							is_set: true,
						}),
					}),
				})}
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
					{...buildArgs({
						onSaveOverride,
						organizations: [MockDefaultOrganization, MockOrganization2],
						selectedOrganization,
						onSelectOrganization: (organization) => {
							onSelectOrganization(organization);
							setSelectedOrganization(organization);
						},
					})}
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
				req: { mode: "model", model_config_id: claudeModelConfig.id },
			},
			expect.objectContaining({ onSuccess: expect.any(Function) }),
		);
	});
});
