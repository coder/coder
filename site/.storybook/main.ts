export default {
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
		// Storybook seems to strip this setting out of our Vite config. We need to
		// put it back in order to be able to access Storybook with Coder Desktop or
		// port sharing.
		config.server = {
			...config.server,
			allowedHosts: [".coder", ".dev.coder.com"],
		};
		return config;
	},
} satisfies import("@storybook/react-vite").StorybookConfig;
