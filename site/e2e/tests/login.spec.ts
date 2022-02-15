import { test } from "@playwright/test"
//import { ResetPasswordPage, SignInPage } from "../../pom"

test("Login takes user to /projects", async ({ page, baseURL }) => {
  console.log("base url: " + baseURL)
  await page.goto(baseURL + "/", { waitUntil: "networkidle" })
})