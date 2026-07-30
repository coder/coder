import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	CoderAgentsPageView,
	type CoderAgentsPageViewProps,
} from "./CoderAgentsPageView";

const buildArgs = (
	overrides: Partial<CoderAgentsPageViewProps> = {},
): CoderAgentsPageViewProps => ({
	adminOverridesData: { allow_users: false },
	adminOverridesError: undefined,
	onRetryAdminOverrides: fn(),
	isRetryingAdminOverrides: false,
	onSaveAdminOverrides: fn(),
	isSavingAdminOverrides: false,
	isSaveAdminOverridesError: false,
	showAdvisorSettings: false,
	advisorConfigData: undefined,
	isAdvisorConfigLoading: false,
	isAdvisorConfigFetching: false,
	isAdvisorConfigLoadError: false,
	onSaveAdvisorConfig: fn(),
	isSavingAdvisorConfig: false,
	isSaveAdvisorConfigError: false,
	saveAdvisorConfigError: undefined,
	showVirtualDesktopSettings: false,
	computerUseProviderData: undefined,
	isLoadingComputerUseProvider: false,
	onSaveComputerUseProvider: fn(),
	isSavingComputerUseProvider: false,
	computerUseProviderSaveError: null,
	...overrides,
});

const getSection = async (
	canvasElement: HTMLElement,
	headingName: string,
): Promise<HTMLElement> => {
	const canvas = within(canvasElement);
	const heading = await canvas.findByRole("heading", { name: headingName });
	const setting = heading.closest("form");
	if (!(setting instanceof HTMLElement)) {
		throw new Error(`Expected ${headingName} heading to live inside a form.`);
	}
	return setting;
};

const meta = {
	title: "pages/AISettingsPage/CoderAgentsPage/CoderAgentsPageView",
	component: CoderAgentsPageView,
	// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
	parameters: { pixel: { exclude: true } },
	args: buildArgs(),
} satisfies Meta<typeof CoderAgentsPageView>;

export default meta;
type Story = StoryObj<typeof CoderAgentsPageView>;

export const Default: Story = {
	args: buildArgs(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("heading", { name: "Coder Agents" }),
		).toBeVisible();
		expect(
			canvas.getByText(
				"Configure deployment-wide defaults for Coder Agents and agent-specific capabilities.",
			),
		).toBeVisible();

		expect(canvas.getByText("Allow personal model overrides")).toBeVisible();
		// Org-scoped model overrides moved to the organization defaults page.
		expect(
			canvas.queryByRole("heading", { name: "General model" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("heading", { name: "Title generation model" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("heading", { name: "Compaction model" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("heading", { name: "Explore subagent model" }),
		).not.toBeInTheDocument();
	},
};

export const PersonalOverridesDisabled: Story = {
	args: buildArgs({
		adminOverridesData: { allow_users: false },
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});

		expect(toggle).not.toBeChecked();
	},
};

export const PersonalOverridesEnabled: Story = {
	args: buildArgs({
		adminOverridesData: { allow_users: true },
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});

		expect(toggle).toBeChecked();
	},
};

export const PersonalOverridesLoadError: Story = {
	args: buildArgs({
		adminOverridesData: undefined,
		adminOverridesError: new Error("Failed to load personal model overrides."),
	}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			await canvas.findByText("Failed to load personal model overrides."),
		).toBeInTheDocument();
		expect(
			canvas.queryByText("Loading personal model override settings..."),
		).not.toBeInTheDocument();
	},
};

export const AdvisorSettingsVisible: Story = {
	args: buildArgs({
		showAdvisorSettings: true,
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 3,
			max_output_tokens: 16384,
		},
	}),
	play: async ({ canvasElement, args }) => {
		const section = await getSection(canvasElement, "Advisor");
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max uses per turn",
			}),
		).toHaveValue(3);
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max output tokens",
			}),
		).toHaveValue(16384);
		// The advisor model picker moved to the organization defaults page.
		expect(within(section).queryByRole("combobox")).not.toBeInTheDocument();

		// Changing a value exposes the Save button.
		const maxUses = within(section).getByRole("spinbutton", {
			name: "Max uses per turn",
		});
		await userEvent.clear(maxUses);
		await userEvent.type(maxUses, "5");
		const saveButton = within(section).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalledWith(
				{ max_uses_per_run: 5, max_output_tokens: 16384 },
				expect.anything(),
			);
		});
	},
};

export const AdvisorClearButton: Story = {
	args: buildArgs({
		showAdvisorSettings: true,
		advisorConfigData: {
			enabled: true,
			max_uses_per_run: 3,
			max_output_tokens: 16384,
		},
	}),
	play: async ({ canvasElement, args }) => {
		const section = await getSection(canvasElement, "Advisor");
		const clearButton = within(section).getByRole("button", { name: "Clear" });
		await userEvent.click(clearButton);
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max uses per turn",
			}),
		).toHaveValue(0);
		expect(
			within(section).getByRole("spinbutton", {
				name: "Max output tokens",
			}),
		).toHaveValue(0);
		const saveButton = within(section).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalledWith(
				{
					max_uses_per_run: 0,
					max_output_tokens: 0,
				},
				expect.anything(),
			);
		});
	},
};

export const VirtualDesktopSettingsVisible: Story = {
	args: buildArgs({
		showVirtualDesktopSettings: true,
		computerUseProviderData: { provider: "anthropic" },
	}),
	play: async ({ canvasElement }) => {
		const section = await getSection(canvasElement, "Virtual desktop");
		expect(
			within(section).getByRole("combobox", {
				name: "Computer use provider",
			}),
		).toHaveTextContent("Anthropic");
	},
};

export const VirtualDesktopProviderChange: Story = {
	args: buildArgs({
		showVirtualDesktopSettings: true,
		computerUseProviderData: { provider: "anthropic" },
	}),
	play: async ({ canvasElement, args }) => {
		const section = await getSection(canvasElement, "Virtual desktop");
		const trigger = within(section).getByRole("combobox", {
			name: "Computer use provider",
		});
		await userEvent.click(trigger);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByRole("option", { name: "OpenAI" }));
		const saveButton = within(section).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveComputerUseProvider).toHaveBeenCalledWith(
				{ provider: "openai" },
				expect.anything(),
			);
		});
	},
};
