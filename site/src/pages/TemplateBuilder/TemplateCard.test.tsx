import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { TemplateCard } from "./TemplateCard";

const renderCards = () => {
	const onSelectDocker = vi.fn();
	const onSelectKubernetes = vi.fn();
	renderComponent(
		<RadioGroup aria-label="Base infrastructure templates" value="">
			<TemplateCard
				value="docker"
				name="Docker Containers"
				description="Provision Docker containers as Coder workspaces."
				detailsUrl="https://registry.coder.com/templates/docker"
				onSelect={onSelectDocker}
			/>
			<TemplateCard
				value="kubernetes"
				name="Kubernetes Pods"
				description="Provision Kubernetes pods as Coder workspaces."
				detailsUrl="https://registry.coder.com/templates/kubernetes"
				onSelect={onSelectKubernetes}
			/>
		</RadioGroup>,
	);
	return { onSelectDocker, onSelectKubernetes };
};

describe(TemplateCard.name, () => {
	it("selects once when the radio is clicked", async () => {
		const { onSelectDocker } = renderCards();
		await userEvent.click(
			screen.getByRole("radio", { name: /Docker Containers/ }),
		);
		expect(onSelectDocker).toHaveBeenCalledTimes(1);
	});

	it("selects once with Space and once per arrow key", async () => {
		const { onSelectDocker, onSelectKubernetes } = renderCards();
		await userEvent.tab();
		expect(
			screen.getByRole("radio", { name: /Docker Containers/ }),
		).toHaveFocus();
		await userEvent.keyboard(" ");
		expect(onSelectDocker).toHaveBeenCalledTimes(1);
		// Hold the key: Radix moves focus in a timeout and only selects on focus
		// while an arrow key is down, so a single press releases it too early.
		await userEvent.keyboard("{ArrowRight>}");
		await waitFor(() => expect(onSelectKubernetes).toHaveBeenCalledTimes(1));
		await userEvent.keyboard("{/ArrowRight}");
		expect(onSelectDocker).toHaveBeenCalledTimes(1);
	});
});
