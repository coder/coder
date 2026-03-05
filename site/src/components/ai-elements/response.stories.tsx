import type { Meta, StoryObj } from "@storybook/react-vite";
import { Response } from "./response";

const sampleMarkdown = `
## Plan update

I checked the auth flow and found two issues:

1. Missing provider fallback for unknown IDs.
2. Error text was not surfaced in the UI.

See [external auth docs](https://coder.com/docs) for expected behavior.

Inline command example: \`git fetch origin\`.

\`\`\`ts
export const ensureProviderLabel = (provider: string) => {
	return provider.trim() || "Git provider";
};
\`\`\`
`;

const sampleFileMarkdown = `
\`\`\`go
package auth

import "errors"

func ValidateToken(token string) error {
	if token == "" {
		return errors.New("token is empty")
	}
	return nil
}
\`\`\`
`;

const meta: Meta<typeof Response> = {
	title: "components/ai-elements/Response",
	component: Response,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		children: sampleMarkdown,
	},
};

export default meta;
type Story = StoryObj<typeof Response>;

export const MarkdownAndLinks: Story = {};

export const FencedFileBlock: Story = {
	args: {
		children: sampleFileMarkdown,
	},
};

export const MarkdownAndLinksLight: Story = {
	globals: {
		theme: "light",
	},
};
