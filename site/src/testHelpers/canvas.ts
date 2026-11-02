import { vi } from "vitest";

// Installs a fake 2D canvas context so code that draws to a canvas, such as
// the web terminal, runs under jsdom. Import this module for its side effect.
// jest-canvas-mock expects a global `jest`.
vi.stubGlobal("jest", vi);
await import("jest-canvas-mock");
