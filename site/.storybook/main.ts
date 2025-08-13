import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
	stories: ["../src/**/*.stories.tsx"],

	addons: [
		"@chromatic-com/storybook",
		"@storybook/addon-docs",
		"@storybook/addon-links",
		"@storybook/addon-themes",
		"storybook-addon-remix-react-router",
	],

	staticDirs: ["../static"],

	framework: {
		name: "@storybook/react-vite",
		options: {},
	},

	async viteFinal(config) {
		if (config.server) {
			config.server.allowedHosts = [".coder"];
		}

		return config;
	},
};

export default config;
