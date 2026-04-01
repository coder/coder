import type { Meta, StoryObj } from "@storybook/react-vite";
import type { SharedWorkspaceActor } from "api/typesGenerated";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { WorkspaceSharingIndicator } from "./WorkspaceSharingIndicator";

const mockUser = (
	name: string,
	roles: SharedWorkspaceActor["roles"] = ["use"],
): SharedWorkspaceActor => ({
	id: crypto.randomUUID(),
	actor_type: "user",
	name,
	roles,
});

const mockGroup = (
	name: string,
	roles: SharedWorkspaceActor["roles"] = ["use"],
): SharedWorkspaceActor => ({
	id: crypto.randomUUID(),
	actor_type: "group",
	name,
	roles,
});

const hoverTrigger = async (canvasElement: HTMLElement) => {
	const trigger = canvasElement.querySelector("svg");
	if (!trigger) {
		throw new Error("Could not find trigger element");
	}
	await userEvent.hover(trigger);
};

const meta: Meta<typeof WorkspaceSharingIndicator> = {
	title: "pages/WorkspacesPage/WorkspaceSharingIndicator",
	component: WorkspaceSharingIndicator,
	args: {
		settingsPath: "/@owner/my-workspace/settings/sharing",
	},
	decorators: [
		(Story) => (
			<div className="p-8">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof WorkspaceSharingIndicator>;

export const SingleUser: Story = {
	args: {
		sharedWith: [mockUser("alice")],
	},
	play: async ({ canvasElement, step }) => {
		await step("activate hover trigger", async () => {
			await hoverTrigger(canvasElement);
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent("alice"),
			);
		});
	},
};

export const SingleAdmin: Story = {
	args: {
		sharedWith: [mockUser("alice", ["admin"])],
	},
	play: async ({ canvasElement, step }) => {
		await step("activate hover trigger", async () => {
			await hoverTrigger(canvasElement);
			await waitFor(() => {
				const tooltip = screen.getByRole("tooltip");
				expect(tooltip).toHaveTextContent("alice");
				expect(tooltip).toHaveTextContent("Admin");
			});
		});
	},
};

export const SingleGroup: Story = {
	args: {
		sharedWith: [mockGroup("Engineering")],
	},
	play: async ({ canvasElement, step }) => {
		await step("activate hover trigger", async () => {
			await hoverTrigger(canvasElement);
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent("Engineering"),
			);
		});
	},
};

export const UsersAndGroups: Story = {
	args: {
		sharedWith: [
			mockGroup("Engineering"),
			mockUser("alice", ["admin"]),
			mockGroup("DevOps", ["admin"]),
			mockUser("bob"),
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("activate hover trigger", async () => {
			await hoverTrigger(canvasElement);
			await waitFor(() => {
				const tooltip = screen.getByRole("tooltip");
				expect(tooltip).toHaveTextContent("alice");
				expect(tooltip).toHaveTextContent("bob");
				expect(tooltip).toHaveTextContent("Engineering");
				expect(tooltip).toHaveTextContent("DevOps");
			});
		});
	},
};

export const ManyActors: Story = {
	args: {
		sharedWith: [
			mockUser("alice", ["admin"]),
			mockUser("bob", ["admin"]),
			mockUser("charlie"),
			mockGroup("QA"),
			mockGroup("HR"),
			mockGroup("Finance"),
			mockGroup("Marketing"),
			mockGroup("Sales"),
			mockUser("david"),
			mockUser("eve"),
			mockUser("frank"),
			mockGroup("Engineering"),
			mockGroup("DevOps"),
			mockGroup("Platform", ["admin"]),
			mockGroup("Security", ["admin"]),
			mockGroup("IT"),
			mockGroup("Legal"),
			mockGroup("Customer Support"),
			mockGroup("Product"),
		],
	},
	play: async ({ canvasElement, step }) => {
		await step("activate hover trigger", async () => {
			await hoverTrigger(canvasElement);
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					"Workspace permissions",
				),
			);
		});
	},
};
