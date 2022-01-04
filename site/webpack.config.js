const path = require('path');

module.exports = {
  entry: './site/index.ts',
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