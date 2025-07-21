import turbosnap from "vite-plugin-turbosnap";

module.exports = {
	stories: ["../src/**/*.stories.tsx"],

	addons: [
		"@chromatic-com/storybook",
		"@storybook/addon-links",
		"@storybook/addon-themes",
		"storybook-addon-remix-react-router",
		"@storybook/addon-docs",
	],

	staticDirs: ["../static"],

	framework: {
		name: "@storybook/react-vite",
		options: {},
	},

	async viteFinal(config, { configType }) {
		config.plugins = config.plugins || [];
		if (configType === "PRODUCTION") {
			config.plugins.push(
				turbosnap({
					rootDir: config.root || "",
				}),
			);
		}
		config.server.allowedHosts = [".coder"];
		return config;
	},
};
