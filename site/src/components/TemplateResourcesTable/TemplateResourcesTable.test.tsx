import { fireEvent, render, screen } from "@testing-library/react"
import { FC } from "react"
import { WrapperComponent } from "../../testHelpers/renderHelpers"
import { Language as AgentTooltipLanguage } from "../Tooltips/AgentHelpTooltip"
import { Language as ResourceTooltipLanguage } from "../Tooltips/ResourcesHelpTooltip"
import { TemplateResourcesProps, TemplateResourcesTable } from "./TemplateResourcesTable"

const Component: FC<React.PropsWithChildren<TemplateResourcesProps>> = (props) => (
  <WrapperComponent>
    <TemplateResourcesTable {...props} />
  </WrapperComponent>
)

describe("TemplateResourcesTable", () => {
  it("displays resources tooltip", () => {
    const props: TemplateResourcesProps = {
      resources: [],
    }
    render(<Component {...props} />)
    const resourceTooltipButton = screen.getAllByRole("button")[0]
    fireEvent.click(resourceTooltipButton)
    const resourceTooltipTitle = screen.getByText(ResourceTooltipLanguage.resourceTooltipTitle)
    expect(resourceTooltipTitle).toBeDefined()
  })
  it("displays agent tooltip", () => {
    const props: TemplateResourcesProps = {
      resources: [],
    }
    render(<Component {...props} />)
    const agentTooltipButton = screen.getAllByRole("button")[1]
    fireEvent.click(agentTooltipButton)
    const agentTooltipTitle = screen.getByText(AgentTooltipLanguage.agentTooltipTitle)
    expect(agentTooltipTitle).toBeDefined()
  })
})
