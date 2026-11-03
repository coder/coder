// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

// Bundlers replace `process.env.NODE_ENV` to remove development-only checks.
declare const process: {
  env: {
    NODE_ENV?: string;
  };
};
