import type { Meta, StoryObj } from "@storybook/react-vite";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { SyntaxHighlighter } from "./SyntaxHighlighter";

const original = `resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}
`;

const modified = `resource "coder_agent" "main" {
  os   = "linux"
  arch = "arm64"
}
`;

const meta: Meta<typeof SyntaxHighlighter> = {
	title: "components/SyntaxHighlighter",
	component: SyntaxHighlighter,
	decorators: [withDashboardProvider],
	args: {
		filename: "main.tf",
	},
};

export default meta;
type Story = StoryObj<typeof SyntaxHighlighter>;

export const Plain: Story = {
	args: {
		value: original,
	},
};

export const Diff: Story = {
	args: {
		value: modified,
		compareWith: original,
	},
};

export const Nix: Story = {
	args: {
		filename: "flake.nix",
		value: `{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  outputs = { self, nixpkgs }: {
    devShells.x86_64-linux.default = nixpkgs.legacyPackages.x86_64-linux.mkShell {
      packages = [ nixpkgs.legacyPackages.x86_64-linux.go ];
    };
  };
}
`,
	},
};
