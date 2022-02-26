import * as path from "path"
import HtmlWebpackPlugin from "html-webpack-plugin"
import * as webpack from "webpack"
import 'webpack-dev-server';

const config: webpack.Configuration = {
  entry: "./index.tsx",
  mode: "production",
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