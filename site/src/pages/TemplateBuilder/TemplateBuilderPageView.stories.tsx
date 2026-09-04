import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type {
	TemplateBuilderBase,
	TemplateBuilderBasesResponse,
} from "#/api/typesGenerated";
import { TemplateBuilderPageView } from "./TemplateBuilderPageView";

const bases: TemplateBuilderBase[] = [
	{
		id: "docker",
		name: "Docker",
		description: "Provision workspaces as Docker containers.",
		icon: "/icon/docker.png",
		os: "linux",
		variables: [],
		prerequisites: "",
		agents: [],
	},
	{
		id: "kubernetes",
		name: "Kubernetes",
		description: "Provision workspaces as Kubernetes pods.",
		icon: "/icon/k8s.png",
		os: "linux",
		variables: [],
		prerequisites: "",
		agents: [],
	},
];

const basesData: TemplateBuilderBasesResponse = { bases };

const meta: Meta<typeof TemplateBuilderPageView> = {
	title: "pages/TemplateBuilder/TemplateBuilderPageView",
	component: TemplateBuilderPageView,
	args: {
		error: null,
		basesData,
		onCreateTemplate: fn(),
		createError: null,
		isCreating: false,
		onClearCreateError: fn(),
		sessionId: "session-1",
	},
	parameters: {
		queries: [
			{
				key: ["templateBuilder", "bases"],
				data: basesData,
			},
			{
				key: ["templateBuilder", "modules", "docker"],
				data: { modules: [] },
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof TemplateBuilderPageView>;

export const Default: Story = {};

// The Continue button stays enabled even when the step is incomplete. Clicking
// it with no base selected reveals a red validation message instead of
// advancing, and the message clears once a base is selected.
export const ContinueShowsValidationError: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const continueButton = await canvas.findByRole("button", {
			name: "Continue",
		});
		await expect(continueButton).toBeEnabled();

		await userEvent.click(continueButton);
		await canvas.findByText("Select a base template to continue.");

		await userEvent.click(await canvas.findByRole("radio", { name: /Docker/ }));
		await waitFor(() =>
			expect(
				canvas.queryByText("Select a base template to continue."),
			).not.toBeInTheDocument(),
		);
	},
};
