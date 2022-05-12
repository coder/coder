import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../testHelpers/renderHelpers"
import { Column, Table } from "./Table"

interface TestData {
  name: string
  description: string
}

const columns: Column<TestData>[] = [
  {
    name: "Name",
    key: "name",
  },
  {
    name: "Description",
    key: "description",
    // For description, we'll test out the custom renderer path
    renderer: (field) => <span>{"!!" + field + "!!"}</span>,
  },
]

const data: TestData[] = [{ name: "AName", description: "ADescription" }]
const emptyData: TestData[] = []

describe("Table", () => {
  it("renders empty state if empty", async () => {
    // Given
    const emptyState = <div>Empty Table!</div>
    const tableProps = {
      title: "TitleTest",
      data: emptyData,
      columns,
      emptyState,
    }

    // When
    render(<Table {...tableProps} />)

    // Then
    // Since there are no items, our empty state should've rendered
    const emptyTextElement = await screen.findByText("Empty Table!")
    expect(emptyTextElement).toBeDefined()
  })

  it("renders title", async () => {
    // Given
    const tableProps = {
      title: "TitleTest",
      data: emptyData,
      columns,
    }

    // When
    render(<Table {...tableProps} />)

    // Then
    const titleElement = await screen.findByText("TitleTest")
    expect(titleElement).toBeDefined()
  })

  it("renders data fields with default renderer if none provided", async () => {
    // Given
    const tableProps = {
      title: "TitleTest",
      data,
      columns,
    }

    // When
    render(<Table {...tableProps} />)

    // Then
    // Check that the 'name' was rendered, with the default renderer
    const nameElement = await screen.findByText("AName")
    expect(nameElement).toBeDefined()
    // ...and the description used our custom rendered
    const descriptionElement = await screen.findByText("!!ADescription!!")
    expect(descriptionElement).toBeDefined()
  })
})
