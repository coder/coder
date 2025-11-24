import { render, screen } from "@testing-library/react";
import { ClassicParameterFlowDeprecationWarning } from "./ClassicParameterFlowDeprecationWarning";

jest.mock("modules/navigation", () => ({
	useLinks: () => () => "/mock-link",
	linkToTemplate: () => "/mock-template-link",
}));

describe("ClassicParameterFlowDeprecationWarning", () => {
	const defaultProps = {
		organizationName: "test-org",
		templateName: "test-template",
	};

	it("renders warning when enabled and user has template update permissions", () => {
		render(
			<ClassicParameterFlowDeprecationWarning
				templateSettingsLink={`/templates/${defaultProps.organizationName}/${defaultProps.templateName}/settings`}
				{...defaultProps}
				isEnabled={true}
			/>,
		);

		expect(screen.getByText("deprecated")).toBeInTheDocument();
		expect(screen.getByText("Go to Template Settings")).toBeInTheDocument();
	});

	it("does not render when enabled is false", () => {
		const { container } = render(
			<ClassicParameterFlowDeprecationWarning
				templateSettingsLink={`/templates/${defaultProps.organizationName}/${defaultProps.templateName}/settings`}
				{...defaultProps}
				isEnabled={false}
			/>,
		);

		expect(container.firstChild).toBeNull();
	});
});
