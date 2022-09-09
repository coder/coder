/**
 * @fileoverview This file contains a production configuration for webpack
 * meant for producing optimized builds.
 */
import CopyWebpackPlugin from "copy-webpack-plugin"
import CSSMinimizerPlugin from "css-minimizer-webpack-plugin"
import MiniCSSExtractPlugin from "mini-css-extract-plugin"
import { Configuration } from "webpack"
import { createCommonWebpackConfig } from "./webpack.common"

const commonWebpackConfig = createCommonWebpackConfig({
  // This decreases compilation time when publishing releases.
  // The "test/js" step will already catch any TypeScript compilation errors.
  skipTypecheck: true,
})

const commonPlugins = commonWebpackConfig.plugins || []

const commonRules = commonWebpackConfig.module?.rules || []

export const config: Configuration = {
  ...commonWebpackConfig,

  mode: "production",

  // Don't produce sourcemaps in production, to minmize bundle size
  devtool: false,

  module: {
    rules: [
      ...commonRules,
      // CSS files -> optimized
      {
        test: /\.css$/i,
        use: [MiniCSSExtractPlugin.loader, "css-loader"],
      },
    ],
  },

  optimization: {
    minimizer: [
      `...`, // This extends the 'default'/'existing' minimizers
      new CSSMinimizerPlugin(),
    ],
  },

  output: {
    ...commonWebpackConfig.output,

    // Regenerate the entire out/ directory (except GITKEEP and out/bin/) when
    // producing production builds. This is important to ensure that old files
    // don't get left behind and embedded in the release binaries.
    clean: {
      keep: /(GITKEEP|bin\/)/,
    },
  },

  plugins: [
    ...commonPlugins,
    // For production builds, we also need to copy all the static
    // files to the 'out' folder.
    new CopyWebpackPlugin({
      patterns: [{ from: "static", to: "." }],
    }),

    // MiniCSSExtractPlugin optimizes CSS
    new MiniCSSExtractPlugin({
      // REMARK: It's important to use [contenthash] here to invalidate caches.
      filename: "[name].[contenthash].css",
      chunkFilename: "[id].css",
    }),
  ],
}

export default config
