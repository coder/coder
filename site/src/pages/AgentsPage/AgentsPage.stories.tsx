import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { fn, spyOn } from "storybook/test";
import { AgentsEmptyState } from "./AgentsPage";

const modelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const meta: Meta<typeof AgentsEmptyState> = {
	title: "pages/AgentsPage/AgentsEmptyState",
	component: AgentsEmptyState,
	args: {
		onCreateChat: fn(),
		isCreating: false,
		createError: undefined,
		modelCatalog: null,
		modelOptions: [...modelOptions],
		isModelCatalogLoading: false,
		modelConfigs: [],
		isModelConfigsLoading: false,
		modelCatalogError: undefined,
	},
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
	},
};

export default meta;
type Story = StoryObj<typeof AgentsEmptyState>;

export const Default: Story = {};
