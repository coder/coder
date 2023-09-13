// Toggle eslint --fix by specifying the `FIX` env.
const fix = !!process.env.FIX;

module.exports = {
  cliOptions: {
    ext: [".js", ".ts", ".tsx"],
    ignorePath: ".eslintignore",
    cache: false,
    fix,
    resolvePluginsRelativeTo: ".",
    maxWarnings: 0,
  },
};
