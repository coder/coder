import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { render } from "#/testHelpers/renderHelpers";
import { RedirectURIsField } from "./RedirectURIsField";

// onChange is a controlled callback, so exercising edit/remove/add needs a
// stateful wrapper to feed each call's result back in as the next `values`.
const ControlledField = ({
	initialValues,
	onChange,
	isPublicClient = false,
}: {
	initialValues: string[];
	onChange: (values: string[]) => void;
	isPublicClient?: boolean;
}) => {
	const [values, setValues] = useState(initialValues);
	return (
		<RedirectURIsField
			values={values}
			disabled={false}
			isPublicClient={isPublicClient}
			onChange={(next) => {
				setValues(next);
				onChange(next);
			}}
			onBlur={() => {}}
		/>
	);
};

describe("RedirectURIsField", () => {
	it("calls onChange with the edited entry", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<ControlledField
				initialValues={["https://a.example.com/cb"]}
				onChange={onChange}
			/>,
		);

		await user.type(screen.getByLabelText(/default callback/i), "2");

		expect(onChange).toHaveBeenLastCalledWith(["https://a.example.com/cb2"]);
	});

	it("calls onChange with the entry removed", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<ControlledField
				initialValues={["https://a.example.com/cb", "https://b.example.com/cb"]}
				onChange={onChange}
			/>,
		);

		await user.click(
			screen.getByRole("button", { name: /remove redirect uri 2/i }),
		);

		expect(onChange).toHaveBeenCalledWith(["https://a.example.com/cb"]);
	});

	it("calls onChange with a new empty entry appended", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<ControlledField
				initialValues={["https://a.example.com/cb"]}
				onChange={onChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /add redirect uri/i }));

		expect(onChange).toHaveBeenCalledWith(["https://a.example.com/cb", ""]);
	});

	it("allows removing the only entry, leaving the list empty", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<ControlledField
				initialValues={["https://a.example.com/cb"]}
				onChange={onChange}
			/>,
		);

		await user.click(
			screen.getByRole("button", { name: /remove redirect uri 1/i }),
		);

		expect(onChange).toHaveBeenCalledWith([]);
	});

	it("disables Add redirect URI once at the max count", () => {
		const values = Array.from(
			{ length: 32 },
			(_, i) => `https://${i}.example.com/cb`,
		);
		render(<ControlledField initialValues={values} onChange={vi.fn()} />);

		expect(
			screen.getByRole("button", { name: /add redirect uri/i }),
		).toBeDisabled();
	});
});
