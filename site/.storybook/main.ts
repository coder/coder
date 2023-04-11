/**
 * @fileoverview This file is configures Storybook
 *
 * @see <https://storybook.js.org/docs/react/configure/overview>
 */
import type { StorybookConfig } from '@storybook/react-vite';
const config: StorybookConfig = {
  // Automatically loads all stories in source ending in 'stories.tsx'
  //
  // SEE: https://storybook.js.org/docs/react/configure/overview#configure-story-loading
  stories: ["../src/**/*.stories.tsx"],
  // addons are official and community plugins to extend Storybook.
  //
  // SEE: https://storybook.js.org/addons
  addons: ["@storybook/addon-links", {
    name: "@storybook/addon-essentials",
    options: {
      actions: false
    }
  },
  ],
  // SEE: https://storybook.js.org/docs/react/configure/babel
  babel: async options => ({
    ...options,
    plugins: [["@babel/plugin-proposal-class-properties", {
      loose: true
    }]]
  }),
  // Static files loaded by storybook, relative to this file.
  //
  // SEE: https://storybook.js.org/docs/react/configure/overview#using-storybook-api
  staticDirs: ["../static"],
  typescript: { check: false },
  framework: {
    name: "@storybook/react-vite",
    options: {}
  },
  features: {
    storyStoreV7: true
  },
  docs: {
    autodocs: true
  }
};
export default config
