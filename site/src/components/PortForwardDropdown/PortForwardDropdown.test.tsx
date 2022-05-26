import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../testHelpers/renderHelpers"
import { Language, PortForwardDropdown } from "./PortForwardDropdown"

const urlFormatter = (port: number | string): string => {
  return `https://${port}--user--workspace.coder.com`
}

describe("PortForwardDropdown", () => {
  it("skips known non-http ports", async () => {
    // When
    const netstat = {
      ports: [
        {
          name: "sshd",
          port: 22,
        },
        {
          name: "code-server",
          port: 8080,
        },
      ],
    }
    render(<PortForwardDropdown urlFormatter={urlFormatter} open netstat={netstat} anchorEl={document.body} />)

    // Then
    let portNameElement = await screen.queryByText(Language.portListing(22, "sshd"))
    expect(portNameElement).toBeNull()
    portNameElement = await screen.findByText(Language.portListing(8080, "code-server"))
    expect(portNameElement).toBeDefined()
  })
})
