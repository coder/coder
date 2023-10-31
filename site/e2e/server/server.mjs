import { chromium } from "@playwright/test";

const server = await chromium.launchServer({ headless: false });
console.log(server.wsEndpoint());
