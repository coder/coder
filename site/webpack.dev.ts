import ReactRefreshWebpackPlugin from "@pmmmwh/react-refresh-webpack-plugin"
import HtmlWebpackPlugin from "html-webpack-plugin"
import * as webpack from "webpack"
import 'webpack-dev-server';

import productionConfig from "./webpack.prod"

const config: webpack.Configuration = {
  ...productionConfig,
  devServer: {
    allowedHosts: "all",
    client: {
      overlay: true,
      progress: false,
      webSocketURL: "auto://0.0.0.0:0/ws"
    },
    devMiddleware: {
      publicPath: "/",
    },
    headers: {
      "Access-Control-Allow-Origin": "*",
    },
    historyApiFallback: {
      index: "index.html",
    },
    hot: true,
    proxy: {
      "/api": "http://localhost:3000",
    },
    static: ["./static"],
  },
  mode: "development",
  plugins: [
    new HtmlWebpackPlugin({
      title: 'Custom template',
      // Load a custom template (lodash by default)
      template: 'index.html',
      inject: "body",
      hash: true,
    }),
    new ReactRefreshWebpackPlugin({
      overlay: true,
    }),
  ],
}

export default config