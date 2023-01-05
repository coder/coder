import { Page } from "@playwright/test"

export const urls = {
  templates: "/templates",
  starterTemplates: "/starter-templates",
  dockerTemplate: "/starter-templates/docker",
  createDockerTemplate: "/templates/new?exampleId=docker"
}

export const buttons = {
  starterTemplates: "Starter templates",
  dockerTemplate: "Develop in Docker",
  useTemplate: "Use template",
  createTemplate: "Create template"
}

export const clickButtonByText = async (page: Page, text: string): Promise<void> => {
  await page.click(`button:has-text("${text}")`)
}
