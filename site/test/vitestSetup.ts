// Vitest setup entry point - imports modular setup files.
import "vitest-axe/extend-expect";
import { expect } from "vitest";
import * as axeMatchers from "vitest-axe/matchers";

import "./setup/polyfills";
import "./setup/domStubs";
import "./setup/mocks";
import "./setup/msw";

expect.extend(axeMatchers);
