import { Page } from "@playwright/test"

export const urls = {
  templates: "/templates",
}

export const buttons = {
  starterTemplates: "Starter templates",
  dockerTemplate: "Develop in Docker",
  useTemplate: "Use template",
  createTemplate: "Create template",
  createWorkspace: "Create workspace",
  submitCreateWorkspace: "Create workspace",
  stopWorkspace: "Stop",
  startWorkspace: "Start"
}

export const clickButtonByText = async (page: Page, text: string): Promise<void> => {
  await page.click(`button:has-text("${text}")`)
}

export const fillInput = async (page: Page, label: string, value: string): Promise<void> => {
  await page.fill(`text=${label}`, value)
}
