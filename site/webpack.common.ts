/**
 * @fileoverview This file contains a set of webpack configurations that should
 * be shared between development and production.
 */

import * as path from "path"
import { Configuration } from "webpack"
import HtmlWebpackPlugin from "html-webpack-plugin"

const templatePath = path.join(__dirname, "html_templates")

const plugins = [
  // The HTML webpack plugin tells webpack to use our `index.html` and inject
  // the bundle script, which might have special naming.
  new HtmlWebpackPlugin({
    template: path.join(templatePath, "index.html"),
    publicPath: "/",
  }),
]

export const commonWebpackConfig: Configuration = {
  // entry defines each "page" or "chunk". Currently, for v2, we only have one bundle -
  // a bundle that is shared across all of the UI. However, we may need to eventually split
  // like in v1, where there is a separate entry piont for dashboard & terminal.
  entry: path.join(__dirname, "src/Main.tsx"),

  // modules specify how different modules are loaded
  // See: https://webpack.js.org/concepts/modules/
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        use: ["ts-loader"],
        exclude: [/node_modules/],
      },
    ],
  },

  resolve: {
    // Let webpack know to consider ts/tsx files for bundling
    // See: https://webpack.js.org/guides/typescript/
    extensions: [".tsx", ".ts", ".js"],
  },

  // output defines the name and location of the final bundle
  output: {
    // The chunk name along with a hash of its content will be used for the
    // generated bundle name.
    //
    // REMARK: It's important to use [contenthash] here to invalidate caches.
    filename: "bundle.[contenthash].js",
    path: path.resolve(__dirname, "out"),
  },

  plugins: plugins,

  target: "web",
}
