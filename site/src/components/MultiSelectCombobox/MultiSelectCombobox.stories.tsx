import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import { MockOrganization, MockOrganization2 } from "testHelpers/entities";
import { MultiSelectCombobox } from "./MultiSelectCombobox";

const organizations = [MockOrganization, MockOrganization2];

const meta: Meta<typeof MultiSelectCombobox> = {
	title: "components/MultiSelectCombobox",
	component: MultiSelectCombobox,
	args: {
		hidePlaceholderWhenSelected: true,
		placeholder: "Select organization",
		emptyIndicator: (
			<p className="text-center text-md text-content-primary">
				All organizations selected
			</p>
		),
		options: organizations.map((org) => ({
			label: org.display_name,
			value: org.id,
		})),
	},
};

export default meta;
type Story = StoryObj<typeof MultiSelectCombobox>;

export const Default: Story = {};

export const OpenCombobox: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));

		await waitFor(() =>
			expect(canvas.getByText("My Organization")).toBeInTheDocument(),
		);
	},
};

export const SelectComboboxItem: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
	},
};

export const SelectAllComboboxItems: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization 2" }),
		);

		await waitFor(() =>
			expect(
				canvas.getByText("All organizations selected"),
			).toBeInTheDocument(),
		);
	},
};

export const ClearFirstSelectedItem: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization 2" }),
		);
		await userEvent.click(canvas.getAllByTestId("clear-option-button")[0]);
	},
};

export const ClearAllComboboxItems: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByPlaceholderText("Select organization"));
		await userEvent.click(
			canvas.getByRole("option", { name: "My Organization" }),
		);
		await userEvent.click(canvas.getByTestId("clear-all-button"));

		await waitFor(() =>
			expect(
				canvas.getByPlaceholderText("Select organization"),
			).toBeInTheDocument(),
		);
	},
};

export const WithIcons: Story = {
	args: {
		placeholder: "Select technology",
		emptyIndicator: (
			<p className="text-center text-md text-content-primary">
				All technologies selected
			</p>
		),
		options: [
			{
				label: "Docker",
				value: "docker",
				icon: "/icon/docker.png",
			},
			{
				label: "Kubernetes",
				value: "kubernetes",
				icon: "/icon/k8s.png",
			},
			{
				label: "VS Code",
				value: "vscode",
				icon: "/icon/code.svg",
			},
			{
				label: "JetBrains",
				value: "jetbrains",
				icon: "/icon/intellij.svg",
			},
			{
				label: "Jupyter",
				value: "jupyter",
				icon: "/icon/jupyter.svg",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Open the combobox
		await userEvent.click(canvas.getByPlaceholderText("Select technology"));

		// Verify that Docker option has an icon
		const dockerOption = canvas.getByRole("option", { name: /Docker/i });
		const dockerIcon = dockerOption.querySelector("img");
		await expect(dockerIcon).toBeInTheDocument();
		await expect(dockerIcon).toHaveAttribute("src", "/icon/docker.png");

		// Select Docker and verify icon appears in badge
		await userEvent.click(dockerOption);

		// Find the Docker badge
		const dockerBadge = canvas
			.getByText("Docker")
			.closest('[role="button"]')?.parentElement;
		const badgeIcon = dockerBadge?.querySelector("img");
		await expect(badgeIcon).toBeInTheDocument();
		await expect(badgeIcon).toHaveAttribute("src", "/icon/docker.png");
	},
};

export const MixedWithAndWithoutIcons: Story = {
	args: {
		placeholder: "Select resource",
		options: [
			{
				label: "CPU",
				value: "cpu",
				icon: "/icon/memory.svg",
			},
			{
				label: "Memory",
				value: "memory",
				icon: "/icon/memory.svg",
			},
			{
				label: "Storage",
				value: "storage",
			},
			{
				label: "Network",
				value: "network",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Open the combobox
		await userEvent.click(canvas.getByPlaceholderText("Select resource"));

		// Verify that CPU option has an icon
		const cpuOption = canvas.getByRole("option", { name: /CPU/i });
		const cpuIcon = cpuOption.querySelector("img");
		await expect(cpuIcon).toBeInTheDocument();

		// Verify that Storage option does not have an icon
		const storageOption = canvas.getByRole("option", { name: /Storage/i });
		const storageIcon = storageOption.querySelector("img");
		await expect(storageIcon).not.toBeInTheDocument();

		// Select both and verify badges
		await userEvent.click(cpuOption);
		await userEvent.click(storageOption);

		// CPU badge should have icon
		const cpuBadge = canvas
			.getByText("CPU")
			.closest('[role="button"]')?.parentElement;
		const cpuBadgeIcon = cpuBadge?.querySelector("img");
		await expect(cpuBadgeIcon).toBeInTheDocument();

		// Storage badge should not have icon
		const storageBadge = canvas
			.getByText("Storage")
			.closest('[role="button"]')?.parentElement;
		const storageBadgeIcon = storageBadge?.querySelector("img");
		await expect(storageBadgeIcon).not.toBeInTheDocument();
	},
};

export const WithGroupedIcons: Story = {
	args: {
		placeholder: "Select tools",
		groupBy: "category",
		options: [
			{
				label: "Docker",
				value: "docker",
				icon: "/icon/docker.png",
				category: "Containers",
			},
			{
				label: "Kubernetes",
				value: "kubernetes",
				icon: "/icon/k8s.png",
				category: "Containers",
			},
			{
				label: "VS Code",
				value: "vscode",
				icon: "/icon/code.svg",
				category: "IDEs",
			},
			{
				label: "JetBrains",
				value: "jetbrains",
				icon: "/icon/intellij.svg",
				category: "IDEs",
			},
			{
				label: "Zed",
				value: "zed",
				icon: "/icon/zed.svg",
				category: "IDEs",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Open the combobox
		await userEvent.click(canvas.getByPlaceholderText("Select tools"));

		// Verify grouped options still have icons
		const dockerOption = canvas.getByRole("option", { name: /Docker/i });
		const dockerIcon = dockerOption.querySelector("img");
		await expect(dockerIcon).toBeInTheDocument();
		await expect(dockerIcon).toHaveAttribute("src", "/icon/docker.png");

		const vscodeOption = canvas.getByRole("option", { name: /VS Code/i });
		const vscodeIcon = vscodeOption.querySelector("img");
		await expect(vscodeIcon).toBeInTheDocument();
		await expect(vscodeIcon).toHaveAttribute("src", "/icon/code.svg");

		// Verify grouping headers are present
		await expect(canvas.getByText("Containers")).toBeInTheDocument();
		await expect(canvas.getByText("IDEs")).toBeInTheDocument();
	},
};
