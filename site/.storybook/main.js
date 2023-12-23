import turbosnap from "vite-plugin-turbosnap";

module.exports = {
  stories: ["../src/**/*.stories.tsx"],
  addons: [
    {
      name: "@storybook/addon-essentials",
      options: {
        backgrounds: false,
      },
    },
    "@storybook/addon-links",
    "@storybook/addon-mdx-gfm",
    "@storybook/addon-actions",
    "@storybook/addon-themes",
  ],
  staticDirs: ["../static"],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  async viteFinal(config, { configType }) {
    config.plugins = config.plugins || [];
    // return the customized config
    if (configType === "PRODUCTION") {
      // ignore @ts-ignore because it's not in the vite types yet
      config.plugins.push(
        turbosnap({
          rootDir: config.root || "",
        }),
      );
    }
    return config;
  },
};
