/**
 * @fileoverview This file contains a production configuration for webpack
 * meant for producing optimized builds.
 */

import CopyWebpackPlugin from "copy-webpack-plugin"
import { Configuration } from "webpack"
import { commonWebpackConfig } from "./webpack.common"

const commonPlugins = commonWebpackConfig.plugins || []

export const config: Configuration = {
  ...commonWebpackConfig,
  mode: "production",

  // Don't produce sourcemaps in production, to minmize bundle size
  devtool: false,

  output: {
    ...commonWebpackConfig.output,

    // regenerate the entire out/ directory when producing production builds
    clean: true,
  },

  plugins: [
    ...commonPlugins,
    // For production builds, we also need to copy all the static
    // files to the 'out' folder.
    new CopyWebpackPlugin({
      patterns: [{ from: "static", to: "." }],
    }),
  ],
}

export default config
