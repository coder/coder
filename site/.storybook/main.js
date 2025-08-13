module.exports = {
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
		config.server.allowedHosts = [".coder"];
		return config;
	},
};
