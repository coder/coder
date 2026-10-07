import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import type { ChatProject } from "#/api/typesGenerated";
import { MockChatProject } from "#/testHelpers/entities";
import { CompactProjectSelector } from "./CompactProjectSelector";

const buildProject = (id: string, name: string, icon = ""): ChatProject => ({
	...MockChatProject,
	id,
	name,
	icon,
});

const projects: ChatProject[] = [
	buildProject("project-launch", "Launch", "/emojis/1f680.png"),
	buildProject("project-incidents", "Incident response", "/emojis/1f525.png"),
	buildProject("project-docs", "Docs", "/emojis/1f4da.png"),
	buildProject("project-plain", "No icon project"),
];

const manyProjects: ChatProject[] = [
	...projects,
	buildProject("project-design", "Design system", "/emojis/1f3a8.png"),
	buildProject("project-tests", "Flaky tests", "/emojis/1f9ea.png"),
	buildProject("project-infra", "Infrastructure", "/emojis/2699-fe0f.png"),
	buildProject("project-bugs", "Bug bash", "/emojis/1f41b.png"),
	buildProject(
		"project-long",
		"A project with a name long enough to be truncated in the list",
	),
	buildProject("project-onboarding", "Onboarding"),
	buildProject("project-billing", "Billing"),
	buildProject("project-research", "Research"),
];

const meta: Meta<typeof CompactProjectSelector> = {
	title: "pages/AgentsPage/ChatElements/CompactProjectSelector",
	component: CompactProjectSelector,
	args: {
		value: null,
		options: projects,
		onChange: fn(),
		onCreateProject: fn(),
		onRetry: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof CompactProjectSelector>;

const openSelector: Story["play"] = async ({ canvasElement }) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button", { name: /^Project:/ }));
	await screen.findByRole("button", { name: "New project" });
};

export const NoProject: Story = {};

export const ProjectSelected: Story = {
	args: { value: projects[0] },
};

export const Disabled: Story = {
	args: { value: projects[0], disabled: true },
};

export const Open: Story = {
	args: { value: projects[1] },
	play: openSelector,
};

export const OpenWithManyProjects: Story = {
	args: { options: manyProjects },
	play: openSelector,
};

export const OpenLoading: Story = {
	args: { options: [], isLoading: true },
	play: openSelector,
};

export const OpenError: Story = {
	args: { options: [], error: new Error("Failed to load projects.") },
	play: openSelector,
};

export const OpenEmpty: Story = {
	args: { options: [] },
	play: openSelector,
};
