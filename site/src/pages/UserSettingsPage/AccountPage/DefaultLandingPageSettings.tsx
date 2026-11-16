import { useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";
import { Section } from "../Section";

type DefaultLandingPageSettingsProps = {
	agentsEnabled: boolean;
};

/** Saves the account-wide home destination independently of profile edits. */
export const DefaultLandingPageSettings: React.FC<
	DefaultLandingPageSettingsProps
> = ({ agentsEnabled }) => {
	const queryClient = useQueryClient();
	const query = useQuery({
		...preferenceSettings(),
		enabled: agentsEnabled,
	});
	const mutation = useMutation(updatePreferenceSettings(queryClient));
	const [open, setOpen] = useState(false);
	const selectId = useId();
	const descriptionId = useId();
	const value = mutation.isPending
		? mutation.variables.default_landing_page
		: (query.data?.default_landing_page ??
			(query.isLoading ? "" : "workspaces"));

	if (!agentsEnabled) {
		return null;
	}

	return (
		<Section title="Dashboard">
			<div className="flex flex-col gap-4">
				<div className="flex flex-col gap-2">
					<Label htmlFor={selectId}>Default landing page</Label>
					<p id={descriptionId} className="m-0 text-sm text-content-secondary">
						Choose your dashboard home page. This preference syncs across your
						devices.
					</p>
					<div className="flex items-center gap-3">
						<Select
							value={value}
							disabled={query.isLoading}
							open={open}
							onOpenChange={(nextOpen) => {
								if (!nextOpen || !mutation.isPending) {
									setOpen(nextOpen);
								}
							}}
							onValueChange={(value) => {
								if (
									mutation.isPending ||
									(value !== "workspaces" && value !== "agents")
								) {
									return;
								}
								mutation.mutate(
									{ default_landing_page: value },
									{
										onSuccess: () =>
											toast.success("Default landing page updated."),
									},
								);
							}}
						>
							<SelectTrigger
								id={selectId}
								className="max-w-xs text-content-primary"
								aria-disabled={mutation.isPending || undefined}
								aria-describedby={descriptionId}
							>
								<SelectValue placeholder="Loading..." />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="workspaces">Workspaces</SelectItem>
								<SelectItem value="agents">Agents</SelectItem>
							</SelectContent>
						</Select>
						<Spinner
							loading={query.isFetching || mutation.isPending}
							label={
								mutation.isPending
									? "Saving default landing page"
									: "Loading default landing page"
							}
							size="sm"
						/>
					</div>
				</div>
				{mutation.isError && <ErrorAlert error={mutation.error} />}
			</div>
		</Section>
	);
};
