/**
 * @fileoverview This file contains a set of webpack configurations that should
 * be shared between development and production.
 */

import HtmlWebpackPlugin from "html-webpack-plugin"
import * as path from "path"
import { Configuration, EnvironmentPlugin } from "webpack"

/**
 * environmentPlugin sets process.env.* variables so that they're available in
 * the application.
 */
const environmentPlugin = new EnvironmentPlugin({
  INSPECT_XSTATE: "",
  CODER_VERSION: "main",
})
console.info(`--- Setting INSPECT_XSTATE to '${process.env.INSPECT_XSTATE || ""}'`)
console.info(`--- Setting CODER_VERSION to '${process.env.CODER_VERSION || "main"}'`)
console.info(`--- Setting NODE_ENV to '${process.env.NODE_ENV || ""}'`)

/**
 * dashboardEntrypoint is the top-most module in the dashboard chunk.
 */
const dashboardEntrypoint = path.join(__dirname, "src/Main.tsx")

/**
 * templatePath is the path to HTML templates for injecting webpack bundles
 */
const templatePath = path.join(__dirname, "htmlTemplates")

/**
 * dashboardHTMLPluginConfig is the HtmlWebpackPlugin configuration for the
 * dashboard chunk.
 */
const dashboardHTMLPluginConfig = new HtmlWebpackPlugin({
  publicPath: "/",
  template: path.join(templatePath, "index.html"),
})

export const commonWebpackConfig: Configuration = {
  // entry defines each "page" or "chunk". In v1, we have two "pages":
  // dashboard and terminal. This is desired because the terminal has the xterm
  // vendor, and it is undesireable to load all of xterm on a dashboard
  // page load.
  //
  // The object key determines the chunk 'name'. This can be used in `output`
  // to create a final bundle name.
  //
  // REMARK: We may need to further vendorize the pieces shared by the chunks
  //         such as React, ReactDOM. This is not yet _optimized_, but having
  //         them split means less initial load on dashboard page load.
  entry: dashboardEntrypoint,

  // output defines the name and location of the final bundle
  output: {
    // The chunk name along with a hash of its content will be used for the
    // generated bundle name.
    //
    // REMARK: It's important to use [contenthash] here to invalidate caches.
    filename: "bundle.[contenthash].js",
    path: path.resolve(__dirname, "out"),
  },

  // modules specify how different modules are loaded
  // See: https://webpack.js.org/concepts/modules/
  module: {
    rules: [
      // TypeScript (ts, tsx) files use ts-loader for simplicity.
      //
      // REMARK: We may want to configure babel-loader later on for further
      //         optimization (build time, tree-shaking). babel-loader on its
      //         own does not run type checks.
      {
        test: /\.tsx?$/,
        use: [
          {
            loader: "ts-loader",
            options: {
              configFile: "tsconfig.prod.json",
            },
          },
        ],
        exclude: /node_modules/,
      },

      // REMARK: webpack 5 asset modules
      {
        test: /\.(png|svg|jpg|jpeg|gif)$/i,
        type: "asset/resource",
      },
    ],
  },

  // resolve extend/modify how modules are resolved.
  //
  // REMARK: Do not add aliases here, unless they cannot be defined in a
  //         tsconfig file (see TSConfigWebpackPlugin).
  resolve: {
    // extensions are attempted in order and enable importing files without
    // the extensions explicitly stated
    //
    // See: https://webpack.js.org/guides/typescript/
    extensions: [".tsx", ".ts", ".js"],
  },

  // plugins customize the build process
  plugins: [environmentPlugin, dashboardHTMLPluginConfig],
}
