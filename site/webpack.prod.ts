/**
 * @fileoverview This file contains a production configuration for webpack
 * meant for producing optimized builds.
 */
import CopyWebpackPlugin from "copy-webpack-plugin"
import CSSMinimizerPlugin from "css-minimizer-webpack-plugin"
import MiniCSSExtractPlugin from "mini-css-extract-plugin"
import { Configuration } from "webpack"
import { commonWebpackConfig } from "./webpack.common"

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

    // regenerate the entire dist/ directory when producing production builds
    clean: true,
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
