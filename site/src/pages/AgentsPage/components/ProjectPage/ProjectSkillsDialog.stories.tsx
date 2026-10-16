import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import {
	chatProjectSkillKey,
	chatProjectSkillsKey,
} from "#/api/queries/chatProjectSkills";
import type { Skill, SkillMetadata } from "#/api/typesGenerated";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { MockDisabledSkill, MockSkills } from "#/testHelpers/skills";
import { ProjectSkillsDialog } from "./ProjectSkillsDialog";

// Relative to now so "Last updated" reads the same in every capture.
const hoursAgo = (hours: number) =>
	new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();

const mockActionSkill: SkillMetadata = {
	...MockSkills[0],
	action: true,
	model_invocable: false,
};

const mockSkills: SkillMetadata[] = [
	mockActionSkill,
	...MockSkills.slice(1),
	MockDisabledSkill,
].map((skill, index) => ({
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
		onCloseAutoFocus: fn(),
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

export const AtCapacity: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: Array.from({ length: 100 }, (_, index) => ({
					...mockSkills[0],
					id: `skill-${index}`,
					name: `skill-${index}`,
				})),
			},
		],
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

/** Back asks before it drops edits that were not saved. */
export const DiscardChanges: Story = {
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Open menu for reviewer" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Edit" }),
		);
		await userEvent.type(
			await screen.findByRole("textbox", { name: "Body" }),
			" Keep it short.",
		);
		await userEvent.click(screen.getByRole("button", { name: "Back" }));
	},
};

export const EditLoading: Story = {
	parameters: {
		queries: [
			{ key: chatProjectSkillsKey(MockChatProject.id), data: mockSkills },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectSkill").mockReturnValue(
			new Promise(() => {}),
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

/** Someone deleted the skill in another tab before this edit was saved. */
export const SaveDeletedError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectSkills").mockResolvedValue(
			mockSkills,
		);
		const notFound = mockApiError({
			message: "Resource not found or you do not have access to this resource.",
		});
		spyOn(API.experimental, "updateChatProjectSkill").mockRejectedValue({
			...notFound,
			response: { ...notFound.response, status: 404 },
		});
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Open menu for reviewer" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Edit" }),
		);
		await userEvent.type(
			await screen.findByRole("textbox", { name: "Body" }),
			" Keep it short.",
		);
		await userEvent.click(screen.getByRole("button", { name: "Save" }));
		await screen.findByText(/was deleted/);
	},
};

/** The API rejects a new skill once the project reaches its limit. */
export const SaveError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectSkills").mockResolvedValue(
			mockSkills,
		);
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
