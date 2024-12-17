import turbosnap from "vite-plugin-turbosnap";

module.exports = {
	stories: ["../src/**/*.stories.tsx"],

	addons: [
		"@chromatic-com/storybook",
		{
			name: "@storybook/addon-essentials",
			options: {
				backgrounds: false,
			},
		},
		"@storybook/addon-links",
		"@storybook/addon-mdx-gfm",
		"@storybook/addon-themes",
		"@storybook/addon-actions",
		"@storybook/addon-interactions",
		"storybook-addon-remix-react-router",
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
		return config;
	},
};
