import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSearchParams } from "react-router";
import { useFilter } from "#/components/Filter/Filter";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { GroupsFilter } from "./GroupsFilter";

type HarnessProps = {
	onChange: (query: string) => void;
};

const Harness: React.FC<HarnessProps> = ({ onChange }) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: onChange,
	});
	return <GroupsFilter filter={filter} />;
};

it("updates and clears group name searches", async () => {
	const user = userEvent.setup();
	const onChange = vi.fn();
	renderWithAuth(<Harness onChange={onChange} />);
	const input = await screen.findByRole("textbox", { name: "Search groups…" });
	await user.type(input, "Platform Engineering");
	await waitFor(() =>
		expect(onChange).toHaveBeenLastCalledWith("Platform Engineering"),
	);
	await user.click(screen.getByRole("button", { name: "Clear search" }));
	await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
});
