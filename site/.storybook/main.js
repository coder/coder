module.exports = {
  stories: ["../src/**/*.stories.tsx"],
  addons: [
    "@storybook/addon-links",
    "@storybook/addon-essentials",
    "@storybook/addon-mdx-gfm",
    "@storybook/addon-actions",
  ],
  staticDirs: ["../static"],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
}
