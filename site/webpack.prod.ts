import * as path from "path"
import CopyWebpackPlugin from "copy-webpack-plugin"
import HtmlWebpackPlugin from "html-webpack-plugin"
import * as webpack from "webpack"
import "webpack-dev-server"

const config: webpack.Configuration = {
  entry: "./Main.tsx",
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
    new CopyWebpackPlugin({
      patterns: [{ from: "static", to: "." }],
    }),
    new HtmlWebpackPlugin({
      title: "Custom template",
      // Load a custom template (lodash by default)
      template: "html_templates/index.html",
      inject: "body",
    }),
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
