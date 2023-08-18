import { test } from "@playwright/test"
import { createTemplate, createWorkspace } from "../helpers"

test("create workspace", async ({ page }) => {
  const template = await createTemplate(page, {
    apply: [
      {
        complete: {
          resources: [
            {
              name: "example",
            },
          ],
        },
      },
    ],
  })
  await createWorkspace(page, template)
})

test("create workspace with default parameters", async ({ page }) => {
  const template = await createTemplate(page, {
    plan: [
      {
        complete: {
          parameters: [
            {
              name: "first_parameter",
              displayName: "First parameter",
              type: "number",
              options: [],
              description: "This is first parameter.",
              icon: "/emojis/1f310.png",
              defaultValue: "123",
              mutable: true,
              required: false,
              order: 1,
              validationRegex: "",
              validationError: "",
              validationMonotonic: "",
            },
            {
              name: "second_parameter",
              displayName: "Second parameter",
              type: "string",
              options: [],
              description: "This is second parameter.",
              defaultValue: "abc",
              icon: "",
              mutable: false,
              required: false,
              order: 2,
              validationRegex: "",
              validationError: "",
              validationMonotonic: "",
            },
            {
              name: "third_parameter",
              displayName: "",
              type: "string",
              options: [],
              description: "This is third parameter.",
              defaultValue: "",
              icon: "",
              mutable: false,
              required: true,
              order: 3,
              validationRegex: "",
              validationError: "",
              validationMonotonic: "",
            },
            {
              name: "fourth_parameter",
              displayName: "",
              type: "bool",
              options: [],
              description: "This is fourth parameter.",
              defaultValue: "true",
              icon: "",
              mutable: false,
              required: true,
              order: 3,
              validationRegex: "",
              validationError: "",
              validationMonotonic: "",
            },
            {
              name: "first_build_option",
              displayName: "First build option",
              type: "bool",
              options: [],
              description: "This is first build option.",
              defaultValue: "false",
              icon: "",
              mutable: true,
              ephemeral: true,
              required: false,
              order: 1,
              validationRegex: "",
              validationError: "",
              validationMonotonic: "",
            }
          ],
        },
      },
    ],
    apply: [
      {
        complete: {
          resources: [
            {
              name: "example",
            },
          ],
        },
      },
    ],
  })
  await createWorkspace(page, template)
})
