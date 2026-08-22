import type { Meta, StoryObj } from "@storybook/react-vite";
import { OAuth2ConsentPageView } from "./OAuth2ConsentPageView";

const meta: Meta<typeof OAuth2ConsentPageView> = {
	title: "pages/OAuth2ConsentPage/Consent",
	component: OAuth2ConsentPageView,
	args: {
		clientName: "Claude Desktop",
		username: "admin",
		redirectUri: "https://claude.ai/api/mcp/auth_callback?state=abc123",
		scopes: [
			"workspace:read",
			"workspace:create",
			"coder:workspaces.operate",
			"template:read",
		],
		onApprove: () => {},
		onDeny: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof OAuth2ConsentPageView>;

export const Default: Story = {};

/** The narrowest useful request: read-only access to workspaces. */
export const ReadOnly: Story = {
	args: {
		clientName: "Workspace status dashboard",
		scopes: ["workspace:read", "template:read", "user:read"],
	},
};

/** Wildcards and delete permissions are labelled, not just coloured. */
export const DestructiveScopes: Story = {
	args: {
		scopes: [
			"workspace:*",
			"workspace:read",
			"user_secret:read",
			"user_secret:delete",
			"api_key:create",
		],
	},
};

/** `all`, or no scope at all, is a full grant — called out rather than listed. */
export const UnrestrictedAccess: Story = {
	args: {
		scopes: ["all"],
	},
};

/** No `scope` parameter: the backend treats this as unrestricted too. */
export const NoScopeRequested: Story = {
	args: {
		scopes: [],
	},
};

/** A scope this deployment's frontend has no description for. */
export const UnrecognizedScope: Story = {
	args: {
		scopes: ["workspace:read", "quantum_widget:entangle"],
	},
};

/** Long lists stay scannable because they're grouped by resource. */
export const ManyScopes: Story = {
	args: {
		scopes: [
			"workspace:read",
			"workspace:create",
			"workspace:start",
			"workspace:stop",
			"workspace:ssh",
			"template:read",
			"template:use",
			"task:read",
			"task:create",
			"file:read",
			"user:read",
			"user:read_personal",
			"api_key:read",
		],
	},
};

export const LongClientName: Story = {
	args: {
		clientName: "Internal integration platform connector (staging environment)",
	},
};

export const Submitting: Story = {
	args: {
		isSubmitting: true,
	},
};
