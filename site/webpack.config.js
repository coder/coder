const path = require("path")

module.exports = {
  devServer: {
    allowedHosts: "all",
    client: {
      overlay: true,
      progress: false,
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
  resolve: {
    extensions: [".tsx", ".ts", ".js"],
  },
  output: {
    filename: "bundle.js",
    path: path.resolve(__dirname, "out"),
  },
  target: "web",
}
