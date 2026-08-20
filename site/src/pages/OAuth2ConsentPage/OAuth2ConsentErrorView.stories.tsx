import type { Meta, StoryObj } from "@storybook/react-vite";
import { OAuth2ConsentErrorView } from "./OAuth2ConsentErrorView";

const meta: Meta<typeof OAuth2ConsentErrorView> = {
	title: "pages/OAuth2ConsentPage/Error",
	component: OAuth2ConsentErrorView,
	args: {
		clientName: "Claude Desktop",
	},
};

export default meta;
type Story = StoryObj<typeof OAuth2ConsentErrorView>;

/** `errUnknownScope` — the scope isn't in the external catalog. */
export const UnknownScope: Story = {
	args: {
		error: "unknown-scope",
		scope: "workspace:teleport",
	},
};

/** `errScopeNotAllowed` — a catalog scope outside the app's allowlist. */
export const ScopeNotAllowed: Story = {
	args: {
		error: "scope-not-allowed",
		scope: "user_secret:read",
	},
};

/** `errNoGrantableScope` — the app's whole allowlist is unsupported here. */
export const NoGrantableScope: Story = {
	args: {
		error: "no-grantable-scope",
	},
};

export const InvalidRedirect: Story = {
	args: {
		error: "invalid-redirect",
	},
};

export const UnknownClient: Story = {
	args: {
		error: "unknown-client",
		clientName: undefined,
	},
};
