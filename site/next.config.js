/* eslint-disable @typescript-eslint/no-unsafe-assignment */
/* eslint-disable @typescript-eslint/no-unsafe-return */
/* eslint-disable @typescript-eslint/no-unsafe-call */
/* eslint-disable @typescript-eslint/no-unsafe-member-access */
/* eslint-disable @typescript-eslint/no-var-requires */

module.exports = {
  env: {},
  experimental: {
    // Allows us to import TS files from outside product/coder/site.
    externalDir: true,
  },
  webpack: (config, { dev, isServer, webpack }) => {
    // Inject CODERD_HOST environment variable for clients
    if (!isServer) {
      config.plugins.push(
        new webpack.DefinePlugin({
          "process.env.CODERD_HOST": JSON.stringify(process.env.CODERD_HOST),
        }),
      )
    }

    return config
  },
}
