import { test, expect } from "@playwright/test"
import { getStatePath } from "../helpers"
import { createTemplate, createTemplateVersion, getOrganization } from "api/api"

test.use({ storageState: getStatePath("authState") })

test("admin can view own workspace", async ({ page, baseURL }) => {
  // import the user response and get the org id off of that
  const { FIRST_USER } = process.env
  const { user_id, organization_id } = JSON.parse(FIRST_USER as any)

  console.log("organizationId", organization_id)

  const org = await getOrganization(organization_id)
  console.log("org", org)

  // const { id } = await createTemplateVersion(organization_id, {
  //   storage_method: "file",
  //   provisioner: "terraform",
  //   tags: {},
  // })

  // console.log("templateVID", id)
  // // Create template
  // await createTemplate(organization_id, {
  //   name: "my-template",
  //   template_version_id: id,
  // })
  // await page.goto(`${baseURL}/templates`, { waitUntil: "networkidle" })
  // await expect(page).toHaveTitle("Templates - Coder")
})
