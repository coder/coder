import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	AgentSettingsExperimentsPageView,
	type AgentSettingsExperimentsPageViewProps,
} from "./AgentSettingsExperimentsPageView";

const baseArgs: AgentSettingsExperimentsPageViewProps = {
	desktopEnabledData: { enable_desktop: false },
	isLoadingDesktopEnabled: false,
	onSaveDesktopEnabled: fn(),
	isSavingDesktopEnabled: false,
	isSaveDesktopEnabledError: false,
	computerUseProviderData: { provider: "anthropic" },
	isLoadingComputerUseProvider: false,
	onSaveComputerUseProvider: fn(),
	isSavingComputerUseProvider: false,
	computerUseProviderSaveError: null,
	debugLoggingData: {
		allow_users: false,
		forced_by_deployment: false,
	},
	isLoadingDebugLogging: false,
	onSaveDebugLogging: fn(),
	isSavingDebugLogging: false,
	isSaveDebugLoggingError: false,
	advisorConfigData: {
		enabled: false,
		max_uses_per_run: 0,
		max_output_tokens: 0,
		reasoning_effort: "",
		model_config_id: "",
	},
	isAdvisorConfigLoading: false,
	isAdvisorConfigFetching: false,
	isAdvisorConfigLoadError: false,
	modelConfigsData: [],
	modelConfigsError: undefined,
	isLoadingModelConfigs: false,
	isFetchingModelConfigs: false,
	onSaveAdvisorConfig: fn(),
	isSavingAdvisorConfig: false,
	isSaveAdvisorConfigError: false,
	saveAdvisorConfigError: undefined,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsExperimentsPageView",
	component: AgentSettingsExperimentsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsExperimentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsExperimentsPageView>;

const getComputerUseProviderSelect = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	return canvas.findByRole("combobox", {
		name: "Computer use provider",
	});
};

const selectComputerUseProvider = async (
	canvasElement: HTMLElement,
	currentSelectionName: string,
	optionName: string,
) => {
	const trigger = await getComputerUseProviderSelect(canvasElement);
	expect(trigger).toHaveTextContent(currentSelectionName);

	await userEvent.click(trigger);
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(await body.findByRole("option", { name: optionName }));
	await waitFor(() => expect(trigger).toHaveTextContent(optionName));
};

function InteractiveComputerUseProviderStory(
	args: AgentSettingsExperimentsPageViewProps,
) {
	const [computerUseProviderData, setComputerUseProviderData] = useState(
		args.computerUseProviderData,
	);

	return (
		<AgentSettingsExperimentsPageView
			{...args}
			computerUseProviderData={computerUseProviderData}
			onSaveComputerUseProvider={(request, options) => {
				if (options) {
					args.onSaveComputerUseProvider(request, options);
				} else {
					args.onSaveComputerUseProvider(request);
				}
				setComputerUseProviderData({ provider: request.provider });
			}}
		/>
	);
}

export const AllowUsersOff: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});

		expect(
			await canvas.findByText("Let users record chat debug logs"),
		).toBeInTheDocument();
		expect(toggle).not.toBeChecked();
	},
};

export const AllowUsersOn: Story = {
	args: {
		debugLoggingData: {
			allow_users: true,
			forced_by_deployment: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});

		expect(toggle).toBeChecked();
	},
};

export const ForcedByDeployment: Story = {
	args: {
		debugLoggingData: {
			allow_users: true,
			forced_by_deployment: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});

		expect(toggle).toBeDisabled();
		expect(
			await canvas.findByText(
				/Debug logging is already enabled deployment-wide/i,
			),
		).toBeInTheDocument();
	},
};

export const DesktopSetting: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Virtual Desktop");
		await canvas.findByText(
			/Allow agents to use a virtual, graphical desktop within workspaces./i,
		);
		await canvas.findByRole("switch", { name: "Enable" });
	},
};

export const VirtualDesktopLoading: Story = {
	args: {
		desktopEnabledData: undefined,
		isLoadingDesktopEnabled: true,
		computerUseProviderData: undefined,
		isLoadingComputerUseProvider: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// While loading, the Switch is replaced by a skeleton placeholder.
		expect(
			canvas.queryByRole("switch", { name: "Enable" }),
		).not.toBeInTheDocument();

		const providerSelect = await getComputerUseProviderSelect(canvasElement);
		expect(providerSelect).toBeDisabled();
	},
};

export const TogglesDesktop: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", { name: "Enable" });

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(args.onSaveDesktopEnabled).toHaveBeenCalledWith({
				enable_desktop: true,
			});
		});
	},
};

export const ComputerUseProviderAnthropic: Story = {
	args: {
		desktopEnabledData: { enable_desktop: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Computer use provider");
		const providerSelect = await getComputerUseProviderSelect(canvasElement);

		expect(providerSelect).not.toBeDisabled();
		expect(providerSelect).toHaveTextContent("Anthropic");
	},
};

export const ComputerUseProviderDisabledWhenDesktopDisabled: Story = {
	play: async ({ canvasElement }) => {
		const providerSelect = await getComputerUseProviderSelect(canvasElement);

		expect(providerSelect).toBeDisabled();
	},
};

export const SelectsOpenAIProvider: Story = {
	args: {
		desktopEnabledData: { enable_desktop: true },
		onSaveComputerUseProvider: fn(),
	},
	render: InteractiveComputerUseProviderStory,
	play: async ({ canvasElement, args }) => {
		await selectComputerUseProvider(canvasElement, "Anthropic", "OpenAI");

		await waitFor(() => {
			expect(args.onSaveComputerUseProvider).toHaveBeenCalledWith({
				provider: "openai",
			});
		});
	},
};

export const SelectsAnthropicProvider: Story = {
	args: {
		desktopEnabledData: { enable_desktop: true },
		computerUseProviderData: { provider: "openai" },
		onSaveComputerUseProvider: fn(),
	},
	render: InteractiveComputerUseProviderStory,
	play: async ({ canvasElement, args }) => {
		await selectComputerUseProvider(canvasElement, "OpenAI", "Anthropic");

		await waitFor(() => {
			expect(args.onSaveComputerUseProvider).toHaveBeenCalledWith({
				provider: "anthropic",
			});
		});
	},
};

export const ComputerUseProviderSaveError: Story = {
	args: {
		computerUseProviderSaveError: new Error("Failed to save."),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to save computer use provider."),
		).toBeInTheDocument();
	},
};

export const ComputerUseProviderSaving: Story = {
	args: {
		desktopEnabledData: { enable_desktop: true },
		isSavingComputerUseProvider: true,
	},
	play: async ({ canvasElement }) => {
		const providerSelect = await getComputerUseProviderSelect(canvasElement);

		expect(providerSelect).toBeDisabled();
	},
};
