import { screen } from "@testing-library/react";
import type { UpdateUserProfileRequest } from "api/typesGenerated";
import { MockUserMember } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { AccountForm } from "./AccountForm";

// NOTE: it does not matter what the role props of MockUser are set to,
//       only that editable is set to true or false. This is passed from
//       the call to /authorization done by auth provider
describe("AccountForm", () => {
	describe("when editable is set to true", () => {
		it("allows updating username", async () => {
			// Given
			const mockInitialValues: UpdateUserProfileRequest = {
				username: MockUserMember.username,
				name: MockUserMember.name,
			};

			// When
			render(
				<AccountForm
					editable
					email={MockUserMember.email}
					initialValues={mockInitialValues}
					isLoading={false}
					onSubmit={() => {
						return;
					}}
				/>,
			);

			// Then
			const el = await screen.findByLabelText("Username");
			expect(el).toBeEnabled();
			const btn = await screen.findByRole("button", {
				name: /Update account/i,
			});
			expect(btn).toBeEnabled();
		});
	});

	describe("when editable is set to false", () => {
		it("does not allow updating username", async () => {
			// Given
			const mockInitialValues: UpdateUserProfileRequest = {
				username: MockUserMember.username,
				name: MockUserMember.name,
			};

			// When
			render(
				<AccountForm
					editable={false}
					email={MockUserMember.email}
					initialValues={mockInitialValues}
					isLoading={false}
					onSubmit={() => {
						return;
					}}
				/>,
			);

			// Then
			const el = await screen.findByLabelText("Username");
			expect(el).toBeDisabled();
		});
	});
});
