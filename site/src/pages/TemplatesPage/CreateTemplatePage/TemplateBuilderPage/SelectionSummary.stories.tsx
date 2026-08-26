import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { SelectionSummary } from "./SelectionSummary";

const meta: Meta<typeof SelectionSummary> = {
	title:
		"pages/TemplatesPage/CreateTemplatePage/TemplateBuilderPage/SelectionSummary",
	component: SelectionSummary,
	args: {
		onNavigateStep: fn(),
		onNavigateModule: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof SelectionSummary>;

export const NoSelection: Story = {
	args: {
		currentStep: 0,
		maxReachedStep: 0,
		selectedTemplate: undefined,
		selectedModules: undefined,
	},
};

export const BaseTemplateStep: Story = {
	args: {
		currentStep: 1,
		maxReachedStep: 1,
		selectedTemplate: undefined,
		selectedModules: undefined,
	},
};

export const WithBaseTemplate: Story = {
	args: {
		currentStep: 1,
		maxReachedStep: 1,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
	},
};

export const ModulesStep: Story = {
	args: {
		currentStep: 2,
		maxReachedStep: 2,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: undefined,
	},
};

export const WithModules: Story = {
	args: {
		currentStep: 2,
		maxReachedStep: 2,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: [
			{
				id: "jetbrains",
				name: "JetBrains",
				iconUrl: "/icon/jetbrains.svg",
			},
			{
				id: "jetbrains-toolbox",
				name: "JetBrains Toolbox",
				iconUrl: "/icon/jetbrains-toolbox.svg",
			},
			{
				id: "cursor",
				name: "Cursor IDE",
				iconUrl: "/icon/cursor.svg",
			},
			{
				id: "claude-code",
				name: "Claude Code",
				iconUrl: "/icon/claude.svg",
			},
			{
				id: "filebrowser",
				name: "File browser",
				iconUrl: "/icon/filebrowser.svg",
			},
			{
				id: "git-clone",
				name: "Git clone",
				iconUrl: "/icon/git.svg",
			},
			{
				id: "devcontainers",
				name: "Devcontainers",
				iconUrl: "/icon/devcontainers.svg",
			},
		],
	},
};

export const WithLongNameModule: Story = {
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
	args: {
		currentStep: 2,
		maxReachedStep: 2,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: [
			{
				id: "git-commit-signing",
				name: "A module with a name long enough to cause the text inside the ModuleSelection component to wrap to the next line, showing that the icon on the left remains top-aligned with the first line of the module name",
				iconUrl: "/icon/git.svg",
			},
		],
	},
};

export const NavigateModuleClick: Story = {
	args: {
		currentStep: 2,
		maxReachedStep: 2,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: [
			{ id: "claude-code", name: "Claude Code", iconUrl: "/icon/claude.svg" },
			{ id: "cursor", name: "Cursor IDE", iconUrl: "/icon/cursor.svg" },
		],
		onNavigateModule: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const moduleButton = await canvas.findByRole("button", {
			name: "Configure Claude Code",
		});
		await userEvent.click(moduleButton);
		await expect(args.onNavigateModule).toHaveBeenCalledWith("claude-code");
	},
};

export const ManyModules: Story = {
	args: {
		currentStep: 2,
		maxReachedStep: 2,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: Array.from({ length: 12 }, (_, i) => ({
			id: `module-${i}`,
			name: `Module ${i + 1}`,
			iconUrl: "/icon/docker.svg",
		})),
	},
};

export const Customizations: Story = {
	args: {
		currentStep: 3,
		maxReachedStep: 3,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: [
			{ id: "claude-code", name: "Claude Code", iconUrl: "/icon/claude.svg" },
			{ id: "cursor", name: "Cursor IDE", iconUrl: "/icon/cursor.svg" },
		],
	},
};

export const NavigationClicks: Story = {
	args: {
		currentStep: 3,
		maxReachedStep: 3,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: [
			{ id: "claude-code", name: "Claude Code", iconUrl: "/icon/claude.svg" },
			{ id: "cursor", name: "Cursor IDE", iconUrl: "/icon/cursor.svg" },
		],
		onNavigateStep: fn(),
		onNavigateModule: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			await canvas.findByRole("button", { name: "Go to Base Template" }),
		);
		await expect(args.onNavigateStep).toHaveBeenCalledWith("base-infra");

		await userEvent.click(
			await canvas.findByRole("button", {
				name: "Configure Docker Containers",
			}),
		);
		await expect(args.onNavigateStep).toHaveBeenCalledWith("base-parameters");

		await userEvent.click(
			await canvas.findByRole("button", { name: "Go to Modules" }),
		);
		await expect(args.onNavigateStep).toHaveBeenCalledWith("module-select");

		await userEvent.click(
			await canvas.findByRole("button", { name: "Go to Customizations" }),
		);
		await expect(args.onNavigateStep).toHaveBeenCalledWith("customizations");

		await userEvent.click(
			await canvas.findByRole("button", { name: "Configure Claude Code" }),
		);
		await expect(args.onNavigateModule).toHaveBeenCalledWith("claude-code");
	},
};

export const BackwardNavigation: Story = {
	// The user reached Customizations (step 3) then jumped back to step 1.
	// Steps 2 and 3 must stay clickable, and both dividers must remain in the
	// completed (green) variant.
	args: {
		currentStep: 1,
		maxReachedStep: 3,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
		selectedModules: [
			{ id: "claude-code", name: "Claude Code", iconUrl: "/icon/claude.svg" },
		],
	},
	play: async ({ canvasElement }) => {
		const dividers = canvasElement.querySelectorAll(
			"[class*='border-border-success']",
		);
		await expect(dividers.length).toBeGreaterThanOrEqual(2);
	},
};

export const UpcomingStepsInert: Story = {
	// On step 1 with nothing selected, steps 2 and 3 must render without a
	// button so they are neither clickable nor focusable.
	args: {
		currentStep: 1,
		maxReachedStep: 1,
		selectedTemplate: undefined,
		selectedModules: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Modules").closest("button")).toBeNull();
		await expect(
			canvas.getByText("Customizations").closest("button"),
		).toBeNull();
	},
};
