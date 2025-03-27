// Import the polyfill first
import 'web-streams-polyfill/polyfill';

import { setupServer } from "msw/node";
import { handlers } from "./handlers";

// This configures a request mocking server with the given request handlers.
export const server = setupServer(...handlers);
