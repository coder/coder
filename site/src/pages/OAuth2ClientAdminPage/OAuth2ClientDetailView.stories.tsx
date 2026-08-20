import type { Meta, StoryObj } from "@storybook/react-vite";
import { OAuth2ClientDetailView } from "./OAuth2ClientDetailView";

const meta: Meta<typeof OAuth2ClientDetailView> = {
	title: "pages/OAuth2ClientAdmin/Detail",
	component: OAuth2ClientDetailView,
	args: {
		name: "Internal deploy bot",
		type: "confidential",
		clientId: "9f2c1b7a-4e55-4c1f-9a2e-6d1f0c3b8a41",
		callbackUrl: "https://deploy.internal.example.com/oauth/callback",
		secrets: [
			{ id: "s1", preview: "••••••••3f2a", lastUsedAt: "2 hours ago" },
			{ id: "s2", preview: "••••••••91c4" },
		],
		onGenerateSecret: () => {},
		onDeleteSecret: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof OAuth2ClientDetailView>;

export const Confidential: Story = {};

/** No secret section at all — replaced by what the client authenticates with instead. */
export const PublicClient: Story = {
	args: {
		name: "Coder CLI",
		type: "public",
		clientId: "3a71f0de-2c94-4f0b-b0d9-51b7c2e4a8d3",
		callbackUrl: "http://localhost:4321/callback",
		secrets: undefined,
		onGenerateSecret: undefined,
	},
};

/** A confidential client that can't authenticate yet. */
export const ConfidentialNoSecrets: Story = {
	args: {
		secrets: [],
	},
};

/** The one moment the full secret exists in the UI. */
export const SecretJustGenerated: Story = {
	args: {
		newSecret: "coder_oauth2_1f9d8c7b6a5e4d3c2b1a0f9e8d7c6b5a",
		secrets: [{ id: "s3", preview: "••••••••6b5a" }],
	},
};
