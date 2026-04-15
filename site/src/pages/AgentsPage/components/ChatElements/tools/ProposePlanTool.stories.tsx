import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
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

const defaultPlanPath =
	"/home/coder/.coder/plans/PLAN-a1b2c3d4-e5f6-7890-abcd-ef1234567890.md";
const defaultPlanFilename = defaultPlanPath.split("/").pop() ?? "PLAN.md";

const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/ProposePlan",
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
	args: { status: "running", args: { path: defaultPlanPath } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(`Proposing ${defaultPlanFilename}…`),
		).toBeInTheDocument();
	},
};

export const Completed: Story = {
	args: {
		status: "completed",
		args: { path: defaultPlanPath },
		result: {
			ok: true,
			path: defaultPlanPath,
			kind: "plan",
			file_id: "test-file-id-completed",
			media_type: "text/markdown",
		},
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockResolvedValue(samplePlan);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Implementation Plan")).toBeInTheDocument();
		expect(
			canvas.getByRole("button", { name: "Copy plan" }),
		).toBeInTheDocument();
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
			file_id: "test-file-id-custom-path",
			media_type: "text/markdown",
		},
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockResolvedValue(samplePlan);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Implementation Plan")).toBeInTheDocument();
		expect(
			canvas.getByRole("button", { name: "Copy plan" }),
		).toBeInTheDocument();
	},
};

export const CompletedCopyButton: Story = {
	args: {
		status: "completed",
		args: { path: "/home/coder/PLAN.md" },
		result: {
			ok: true,
			path: "/home/coder/PLAN.md",
			kind: "plan",
			file_id: "test-file-id-copy",
			media_type: "text/markdown",
		},
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockResolvedValue(samplePlan);
		spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Implementation Plan");
		const copyBtn = canvas.getByRole("button", { name: "Copy plan" });
		await userEvent.click(copyBtn);
		expect(navigator.clipboard.writeText).toHaveBeenCalledWith(samplePlan);
	},
};

export const ErrorState: Story = {
	args: {
		status: "completed",
		isError: true,
		args: { path: defaultPlanPath },
		result: "Failed to read file: file not found",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(`Proposed ${defaultPlanFilename}`),
		).toBeInTheDocument();
		expect(canvas.getByLabelText("Error")).toBeInTheDocument();
	},
};

export const EmptyContent: Story = {
	args: {
		status: "completed",
		args: { path: defaultPlanPath },
		result: {
			ok: true,
			path: defaultPlanPath,
			kind: "plan",
			file_id: "test-file-id-empty-content",
			media_type: "text/markdown",
		},
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockResolvedValue("");
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("No plan content.")).toBeInTheDocument();
	},
};

export const FileIDLoading: Story = {
	args: {
		status: "completed",
		args: { path: defaultPlanPath },
		result: {
			ok: true,
			path: defaultPlanPath,
			kind: "plan",
			file_id: "test-file-id-loading",
			media_type: "text/markdown",
		},
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText(/Loading plan/)).toBeInTheDocument();
	},
};

export const FileIDCompleted: Story = {
	args: {
		status: "completed",
		args: { path: defaultPlanPath },
		result: {
			ok: true,
			path: defaultPlanPath,
			kind: "plan",
			file_id: "test-file-id-success",
			media_type: "text/markdown",
		},
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockResolvedValue(samplePlan);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Implementation Plan")).toBeInTheDocument();
	},
};

export const FileIDFetchError: Story = {
	args: {
		status: "completed",
		args: { path: defaultPlanPath },
		result: {
			ok: true,
			path: defaultPlanPath,
			kind: "plan",
			file_id: "test-file-id-error",
			media_type: "text/markdown",
		},
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatFileText").mockRejectedValue(
			new Error("Failed to load plan"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByLabelText("Error")).toBeInTheDocument();
	},
};
