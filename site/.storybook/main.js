const path = require("path")

module.exports = {
  stories: ["../src/**/*.stories.mdx", "../src/**/*.stories.@(js|jsx|ts|tsx)"],
  addons: ["@storybook/addon-links", "@storybook/addon-essentials"],
  babel: async (options) => ({
    ...options,
    plugins: ["@babel/plugin-proposal-class-properties"],
    // any extra options you want to set
  }),
  webpackFinal: async (config) => {
    config.resolve.modules = [path.resolve(__dirname, ".."), "node_modules"]

    return config
  },
}
