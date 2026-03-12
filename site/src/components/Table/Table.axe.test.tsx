import "vitest-axe/extend-expect";
import { render } from "@testing-library/react";
import { expect } from "vitest";
import { axe } from "vitest-axe";
import * as axeMatchers from "vitest-axe/matchers";

expect.extend(axeMatchers);

import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "./Table";

describe("Table", () => {
	it("has no axe violations", async () => {
		const { container } = render(
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Value</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					<TableRow>
						<TableCell>Foo</TableCell>
						<TableCell>Bar</TableCell>
					</TableRow>
				</TableBody>
			</Table>,
		);

		const results = await axe(container);
		expect(results).toHaveNoViolations();
	});
});
