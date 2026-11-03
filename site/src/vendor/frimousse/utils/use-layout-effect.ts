// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import { useEffect, useLayoutEffect as useOriginalLayoutEffect } from "react";

// On React 18.2.0 and earlier, useLayoutEffect triggers a warning when executed on the server
export const useLayoutEffect =
  /* v8 ignore next */
  typeof window !== "undefined" ? useOriginalLayoutEffect : useEffect;
