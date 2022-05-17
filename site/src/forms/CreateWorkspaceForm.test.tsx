import { render, screen } from "@testing-library/react"
import React from "react"
import { reach, StringSchema } from "yup"
import { MockOrganization, MockTemplate, MockWorkspace } from "../testHelpers/renderHelpers"
import { CreateWorkspaceForm, validationSchema } from "./CreateWorkspaceForm"

const nameSchema = reach(validationSchema, "name") as StringSchema

describe("CreateWorkspaceForm", () => {
  it("renders", async () => {
    // Given
    const onSubmit = () => Promise.resolve(MockWorkspace)
    const onCancel = () => Promise.resolve()

    // When
    render(
      <CreateWorkspaceForm
        template={MockTemplate}
        onSubmit={onSubmit}
        onCancel={onCancel}
        organizationId={MockOrganization.id}
      />,
    )

    // Then
    // Simple smoke test to verify form renders
    const element = await screen.findByText("Create Workspace")
    expect(element).toBeDefined()
  })

  describe("validationSchema", () => {
    it("allows a 1-letter name", () => {
      const validate = () => nameSchema.validateSync("t")
      expect(validate).not.toThrow()
    })

    it("allows a 32-letter name", () => {
      const input = Array(32).fill("a").join("")
      const validate = () => nameSchema.validateSync(input)
      expect(validate).not.toThrow()
    })

    it("allows 'test-3' to be used as name", () => {
      const validate = () => nameSchema.validateSync("test-3")
      expect(validate).not.toThrow()
    })

    it("allows '3-test' to be used as a name", () => {
      const validate = () => nameSchema.validateSync("3-test")
      expect(validate).not.toThrow()
    })

    it("disallows a 33-letter name", () => {
      const input = Array(33).fill("a").join("")
      const validate = () => nameSchema.validateSync(input)
      expect(validate).toThrow()
    })

    it("disallows a space", () => {
      const validate = () => nameSchema.validateSync("test 3")
      expect(validate).toThrow()
    })
  })
})
