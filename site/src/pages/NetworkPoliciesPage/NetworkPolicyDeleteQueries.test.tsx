import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	QueryClient,
	QueryClientProvider,
	useMutation,
	useQueryClient,
} from "react-query";
import { API } from "#/api/api";
import {
	deleteNetworkPolicy,
	networkPoliciesKey,
	networkPolicyKey,
} from "#/api/queries/networkPolicies";
import { MockNetworkPolicy } from "#/testHelpers/networkPolicies";
import { NetworkPolicyDelete } from "./NetworkPolicyDelete";

const DeletePolicy: React.FC = () => {
	const queryClient = useQueryClient();
	const remove = useMutation(
		deleteNetworkPolicy(queryClient, "default", MockNetworkPolicy.id),
	);
	return (
		<NetworkPolicyDelete
			name={MockNetworkPolicy.name}
			generation={MockNetworkPolicy.generation}
			deleted={false}
			isPending={remove.isPending}
			blockedReason={undefined}
			error={remove.error}
			onDelete={(request, onSuccess) => remove.mutate(request, { onSuccess })}
		/>
	);
};

it.each([false, true])(
	"invalidates deletion progress and the list even after a failed response (%s)",
	async (failed) => {
		const remove = vi.spyOn(API.experimental, "deleteNetworkPolicy");
		if (failed)
			remove.mockRejectedValue({
				message: "The policy changed. Reload before deleting.",
			});
		else
			remove.mockResolvedValue({
				...MockNetworkPolicy,
				deletion_requested_at: "2026-10-21T12:00:00Z",
			});
		const queryClient = new QueryClient({
			defaultOptions: { mutations: { retry: false } },
		});
		queryClient.setQueryData(networkPoliciesKey("default"), [
			MockNetworkPolicy,
		]);
		queryClient.setQueryData(
			networkPolicyKey("default", MockNetworkPolicy.id),
			MockNetworkPolicy,
		);
		render(
			<QueryClientProvider client={queryClient}>
				<DeletePolicy />
			</QueryClientProvider>,
		);
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: "Delete policy" }));
		await user.click(screen.getByRole("button", { name: "Delete" }));
		await waitFor(() =>
			expect(remove).toHaveBeenCalledExactlyOnceWith(
				"default",
				MockNetworkPolicy.id,
				{ expected_generation: MockNetworkPolicy.generation },
			),
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
