const path = require('path');

module.exports = {
  entry: './site/index.tsx',
  mode: "development",
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        use: 'ts-loader',
        exclude: /node_modules/,
      },
    ],
  },
  resolve: {
    extensions: ['.tsx', '.ts', '.js'],
    alias: {
      '@v1': path.resolve(__dirname, '..', 'm'),
      'lib/coderapi': path.resolve(__dirname, '..', 'm', 'lib', 'ts', 'coderapi', 'src')
    },
    modules: [
      'node_modules',
      path.resolve(__dirname, '..', 'm'),
      path.resolve(__dirname, '..', 'm', 'lib'),
      path.resolve(__dirname, '..', 'm', 'product', 'coder', 'site')]
  },
  output: {
    filename: 'bundle.js',
    path: path.resolve(__dirname, 'dist'),
  },
};