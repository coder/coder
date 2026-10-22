import "#/testHelpers/monaco";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import {
	networkPoliciesKey,
	networkPolicyKey,
} from "#/api/queries/networkPolicies";
import { MockTemplate } from "#/testHelpers/entities";
import {
	MockNetworkPolicy,
	MockNetworkPolicyLogs,
	MockNetworkPolicyRevision,
	MockNetworkPolicyRun,
} from "#/testHelpers/networkPolicies";
import { render } from "#/testHelpers/renderHelpers";
import { NetworkPolicyDetail } from "./NetworkPolicyDetail";
import { NetworkPolicyRunLogs } from "./NetworkPolicyRunLogs";

it.each([false, true])(
	"invalidates policy and list after a revision save, including conflicts (%s)",
	async (conflict) => {
		const save = vi.spyOn(API.experimental, "createNetworkPolicyRevision");
		if (conflict)
			save.mockRejectedValue({
				message: "Policy changed. Reload before saving.",
			});
		else save.mockResolvedValue(MockNetworkPolicyRevision);
		const { queryClient } = render(
			<NetworkPolicyDetail
				organization="default"
				policy={MockNetworkPolicy}
				templates={[MockTemplate]}
			/>,
		);
		queryClient.setQueryDefaults(networkPoliciesKey("default"), {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(networkPoliciesKey("default"), [
			MockNetworkPolicy,
		]);
		queryClient.setQueryData(
			networkPolicyKey("default", MockNetworkPolicy.id),
			MockNetworkPolicy,
		);
		const user = userEvent.setup();
		await user.clear(screen.getByRole("textbox", { name: "Policy HCL" }));
		await user.type(
			screen.getByRole("textbox", { name: "Policy HCL" }),
			"updated HCL",
		);
		await user.click(screen.getByRole("button", { name: "Save new version" }));
		await waitFor(() =>
			expect(save).toHaveBeenCalledWith("default", MockNetworkPolicy.id, {
				hcl: "updated HCL",
				expected_generation: MockNetworkPolicy.generation,
			}),
		);
		await waitFor(() =>
			expect(
				queryClient.getQueryState(
					networkPolicyKey("default", MockNetworkPolicy.id),
				)?.isInvalidated,
			).toBe(true),
		);
		expect(
			queryClient.getQueryState(networkPoliciesKey("default"))?.isInvalidated,
		).toBe(true);
	},
);

it("retries scoped persisted run logs after a request failure", async () => {
	const logs = vi
		.spyOn(API.experimental, "getNetworkPolicyRunLogs")
		.mockRejectedValueOnce({ message: "Logs unavailable" })
		.mockResolvedValue(MockNetworkPolicyLogs);
	render(
		<NetworkPolicyRunLogs
			organization="default"
			policyId={MockNetworkPolicy.id}
			runId={MockNetworkPolicyRun.id}
		/>,
	);
	await userEvent
		.setup()
		.click(await screen.findByRole("button", { name: "Retry logs" }));
	await waitFor(() => expect(logs).toHaveBeenCalledTimes(2));
	expect(logs).toHaveBeenLastCalledWith(
		"default",
		MockNetworkPolicy.id,
		MockNetworkPolicyRun.id,
	);
});
