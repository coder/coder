import { test, expect as chromaticExpect } from "@chromatic/test";
import { mergeExpects } from "@playwright/test";
import { expectUrl } from "./expectUrl";

const mergedExpect = mergeExpects(chromaticExpect, expectUrl);
export { mergedExpect as expect };
export { test };

export { chromium, firefox, type Page, webkit } from "@playwright/test";
