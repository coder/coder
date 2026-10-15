import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import {
	chatProjectSkillKey,
	chatProjectSkillsKey,
} from "#/api/queries/chatProjectSkills";
import type { Skill, SkillMetadata } from "#/api/typesGenerated";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { MockSkills } from "#/testHelpers/skills";
import { ProjectSkillsDialog } from "./ProjectSkillsDialog";

// Relative to now so "Last updated" reads the same in every capture.
const hoursAgo = (hours: number) =>
	new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();

const mockSkills: SkillMetadata[] = MockSkills.map((skill, index) => ({
	...skill,
	updated_at: hoursAgo(index * 5 + 2),
}));

const mockReviewerSkill: Skill = {
	...mockSkills[0],
	content:
		'---\nname: reviewer\ndescription: "Review changed files and suggest fixes."\n---\nRead the diff, then list concrete fixes with file paths.\n',
};

const meta: Meta<typeof ProjectSkillsDialog> = {
	title: "pages/AgentsPage/ProjectPage/ProjectSkillsDialog",
	component: ProjectSkillsDialog,
	args: {
		project: MockChatProject,
		open: true,
		onOpenChange: fn(),
		canEdit: true,
	},
	parameters: {
		queries: [
			{ key: chatProjectSkillsKey(MockChatProject.id), data: mockSkills },
			{
				key: chatProjectSkillKey(MockChatProject.id, mockReviewerSkill.name),
				data: mockReviewerSkill,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ProjectSkillsDialog>;

export const List: Story = {};

export const Empty: Story = {
	parameters: {
		queries: [{ key: chatProjectSkillsKey(MockChatProject.id), data: [] }],
	},
};

export const Loading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectSkills").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const LoadError: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectSkills").mockRejectedValue(
			mockApiError({ message: "Failed to list project skills." }),
		);
	},
};

export const New: Story = {
	play: async () => {
		await userEvent.click(await screen.findByRole("button", { name: "New" }));
	},
};

export const Edit: Story = {
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Open menu for reviewer" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Edit" }),
		);
	},
};

/** The saved skill could not be loaded into the editor. */
export const EditLoadError: Story = {
	parameters: {
		queries: [
			{ key: chatProjectSkillsKey(MockChatProject.id), data: mockSkills },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectSkill").mockRejectedValue(
			mockApiError({ message: "Failed to get project skill." }),
		);
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Open menu for reviewer" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Edit" }),
		);
	},
};

export const ReadOnlyList: Story = {
	args: { canEdit: false },
};

export const ReadOnlyView: Story = {
	args: { canEdit: false },
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Open menu for reviewer" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "View" }),
		);
	},
};

/** The API rejects a new skill once the project reaches its limit. */
export const SaveError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "createChatProjectSkill").mockRejectedValue(
			mockApiError({
				message: "Project skill limit reached.",
				detail: "Each project can have at most 100 skills.",
			}),
		);
	},
	play: async () => {
		await userEvent.click(await screen.findByRole("button", { name: "New" }));
		await userEvent.type(
			screen.getByRole("textbox", { name: "Name" }),
			"planner",
		);
		await userEvent.type(
			screen.getByRole("textbox", { name: "Body" }),
			"Plan the work before writing code.",
		);
		await userEvent.click(screen.getByRole("button", { name: "Save" }));
		await screen.findByText("Project skill limit reached.");
	},
};
