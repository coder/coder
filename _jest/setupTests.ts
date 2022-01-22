/**
 * Global setup for our Jest tests
 */

// Set up 'next-router-mock' to with our front-end tests:
// https://github.com/scottrippey/next-router-mock#quick-start
jest.mock("next/router", () => require("next-router-mock"))
