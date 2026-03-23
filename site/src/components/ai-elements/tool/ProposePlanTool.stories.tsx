import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { Tool } from "./Tool";

const samplePlan = [
	"# Implementation Plan",
	"",
	"## Goal",
	"Refactor the authentication module to support OAuth2 providers.",
	"",
	"## Steps",
	"",
	"### 1. Database migrations",
	"- [ ] Add `oauth2_providers` table",
	"- [x] Update `users` table with provider column",
	"",
	"### 2. Backend",
	"```go",
	"type OAuth2Provider struct {",
	"    ID   uuid.UUID",
	"    Name string",
	"}",
	"```",
	"",
	"### 3. API endpoints",
	"- `GET /api/v2/oauth2/providers`",
	"- `POST /api/v2/oauth2/callback`",
	"",
	"## Acceptance criteria",
	"1. Users can authenticate via OAuth2",
	"2. Existing password auth continues to work",
	"",
	"> **Note**: Based on [RFC 6749](https://tools.ietf.org/html/rfc6749).",
].join("\n");

const meta: Meta<typeof Tool> = {
	title: "components/ai-elements/tool/ProposePlan",
	component: Tool,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: { name: "propose_plan" },
	parameters: {
		reactRouter: reactRouterParameters({ routing: { path: "/" } }),
	},
};
export default meta;
type Story = StoryObj<typeof Tool>;

export const Running: Story = {
	args: { status: "running", args: { path: "/home/coder/PLAN.md" } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Proposing PLAN\.md/)).toBeInTheDocument();
	},
};

export const Completed: Story = {
	args: {
		status: "completed",
		args: { path: "/home/coder/PLAN.md" },
		result: {
			ok: true,
			path: "/home/coder/PLAN.md",
			kind: "plan",
			content: samplePlan,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Proposed PLAN\.md/)).toBeInTheDocument();
		expect(canvas.getByText("Implementation Plan")).toBeInTheDocument();
		const toggle = canvas.getByRole("button");
		expect(toggle).toHaveAttribute("aria-expanded", "true");
	},
};

export const CustomPath: Story = {
	args: {
		status: "completed",
		args: { path: "/home/coder/docs/AUTH_PLAN.md" },
		result: {
			ok: true,
			path: "/home/coder/docs/AUTH_PLAN.md",
			kind: "plan",
			content: samplePlan,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Proposed AUTH_PLAN\.md/)).toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	args: {
		status: "completed",
		isError: true,
		args: { path: "/home/coder/PLAN.md" },
		result: "Failed to read file: file not found",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Proposed PLAN\.md/)).toBeInTheDocument();
		expect(canvas.getByLabelText("Error")).toBeInTheDocument();
	},
};

export const EmptyContent: Story = {
	args: {
		status: "completed",
		args: { path: "/home/coder/PLAN.md" },
		result: {
			ok: true,
			path: "/home/coder/PLAN.md",
			kind: "plan",
			content: "",
		},
	},
};

export const CollapseToggle: Story = {
	args: {
		status: "completed",
		args: { path: "/home/coder/PLAN.md" },
		result: {
			ok: true,
			path: "/home/coder/PLAN.md",
			kind: "plan",
			content: samplePlan,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button");
		expect(toggle).toHaveAttribute("aria-expanded", "true");
		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "true");
	},
};
