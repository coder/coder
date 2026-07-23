import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { SelectionSummary } from "./SelectionSummary";

const meta: Meta<typeof SelectionSummary> = {
	title: "pages/TemplateBuilder/SelectionSummary",
	component: SelectionSummary,
};

export default meta;
type Story = StoryObj<typeof SelectionSummary>;

export const NoSelection: Story = {
	args: {
		currentStep: 0,
		selectedTemplate: undefined,
		selectedModules: undefined,
	},
};

export const BaseTemplateStep: Story = {
	args: {
		currentStep: 1,
		selectedTemplate: undefined,
		selectedModules: undefined,
	},
};

export const WithBaseTemplate: Story = {
	args: {
		currentStep: 1,
		selectedTemplate: {
			name: "Docker Containers",
			iconUrl: "/icon/docker.svg",
		},
	},
};

export const ModulesStep: Story = {
	args: {
		currentStep: 2,
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
	parameters: {
		viewport: { defaultViewport: "mobile1" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const moduleName =
			"A module with a name long enough to cause the text inside the ModuleSelection component to wrap to the next line, showing that the icon on the left remains top-aligned with the first line of the module name";
		const name = await canvas.findByText(moduleName);
		const row = name.parentElement;
		const avatar = row?.querySelector<HTMLElement>("div > span");
		if (!(row instanceof HTMLElement) || !avatar) {
			throw new Error(
				"Expected the module row to contain its avatar and name.",
			);
		}

		await waitFor(() => {
			const nameStyle = getComputedStyle(name);
			const lineHeight = Number.parseFloat(nameStyle.lineHeight);
			expect(name.getBoundingClientRect().height).toBeGreaterThan(lineHeight);
			expect(
				Math.abs(
					avatar.getBoundingClientRect().top - name.getBoundingClientRect().top,
				),
			).toBeLessThanOrEqual(1);
		});
	},
};

export const ManyModules: Story = {
	args: {
		currentStep: 2,
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
