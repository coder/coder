/**
 * @fileoverview This file contains a development configuration for webpack
 * meant for webpack-dev-server.
 */
import ReactRefreshWebpackPlugin from "@pmmmwh/react-refresh-webpack-plugin"
import { Configuration } from "webpack"
import "webpack-dev-server"
import { commonWebpackConfig } from "./webpack.common"

const commonPlugins = commonWebpackConfig.plugins || []

const commonRules = commonWebpackConfig.module?.rules || []

const config: Configuration = {
  ...commonWebpackConfig,

  // devtool controls how source maps are generated. In development, we want
  // more details (less optimized) for more readability and an easier time
  // debugging
  devtool: "eval-source-map",

  // devServer is the configuration for webpack-dev-server.
  //
  // REMARK: needs webpack-dev-server import at top of file for typings
  devServer: {
    // allowedHosts are services that can access the running server.
    // Setting allowedHosts sets up the development server to spend specific headers to allow cross-origin requests.
    // In v1, we use CODERD_HOST for the allowed host and origin in order to mitigate security risks.
    // We don't have an equivalent in v2 - but we can allow localhost and cdr.dev,
    // so that the site is accessible through dev urls.
    // We don't want to use 'all' or '*' and risk a security hole in our dev environments.
    allowedHosts: ["localhost", ".cdr.dev"],

    // client configures options that are observed in the browser/web-client.
    client: {
      // automatically sets the browser console to verbose when using HMR
      logging: "verbose",

      // errors will display as a full-screen overlay
      overlay: true,

      // build % progress will display in the browser
      progress: true,

      // webpack-dev-server uses a webSocket to communicate with the browser
      // for HMR. By setting this to auto://0.0.0.0/ws, we allow the browser
      // to set the protocal, hostname and port automatically for us.
      webSocketURL: "auto://0.0.0.0:0/ws",
    },
    devMiddleware: {
      publicPath: "/",
    },
    headers: {
      "Access-Control-Allow-Origin": "*",
    },

    // historyApiFallback is required when using history (react-router) for
    // properly serving index.html on 404s.
    historyApiFallback: true,
    hot: true,
    port: process.env.PORT || 8080,
    proxy: {
      "/api": {
        target: "http://localhost:3000",
        ws: true,
        secure: false,
      },
    },
    static: ["./static"],
  },

  // Development mode - see:
  // https://webpack.js.org/configuration/mode/#mode-development
  mode: "development",

  module: {
    rules: [
      ...commonRules,

      {
        test: /\.css$/i,
        // Use simple style-loader for CSS modules. This places styles directly
        // in <style> tags which is great for development, but poor for loading
        // in production
        use: ["style-loader", "css-loader"],
      },
    ],
  },

  output: {
    ...commonWebpackConfig.output,

    // The chunk name will be used as-is for the bundle output
    // This is simpler than production, to improve performance
    // (no need to calculate hashes in development)
    filename: "[name].js",
  },

  plugins: [
    ...commonPlugins,

    // The ReactRefreshWebpackPlugin enables hot-module reloading:
    // https://github.com/pmmmwh/react-refresh-webpack-plugin
    new ReactRefreshWebpackPlugin({
      overlay: true,
    }),
  ],
}

export default config
