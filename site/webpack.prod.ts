import * as path from "path"
import HtmlWebpackPlugin from "html-webpack-plugin"
import * as webpack from "webpack"
import 'webpack-dev-server';

export const commonPlugins = [

]

const config: webpack.Configuration = {
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
  entry: "./index.tsx",
  // TODO: 
  mode: "development",
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        use: ["ts-loader"],
        exclude: [/node_modules/],
      },
    ],
  },
  plugins: [
    new HtmlWebpackPlugin({
      title: 'Custom template',
      // Load a custom template (lodash by default)
      template: 'index.html',
      inject: "body"
    })
  ],
  resolve: {
    extensions: [".tsx", ".ts", ".js"],
  },
  output: {
    filename: "bundle.[contenthash].js",
    path: path.resolve(__dirname, "out"),
  },
  target: "web",
}

export default config