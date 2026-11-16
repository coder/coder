import { useEffect, useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
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
	const query = useQuery(preferenceSettings());
	const mutation = useMutation(updatePreferenceSettings(queryClient));
	const [open, setOpen] = useState(false);
	const triggerRef = useRef<HTMLButtonElement>(null);
	const focusAfterRetry = useRef(false);
	const selectId = useId();
	const descriptionId = useId();
	const availabilityId = useId();
	const value = query.data
		? mutation.isPending
			? mutation.variables.default_landing_page
			: query.data.default_landing_page
		: "";
	const error = mutation.error || query.error;

	useEffect(() => {
		if (
			query.isFetching ||
			!focusAfterRetry.current ||
			(!query.data && query.errorUpdatedAt === 0)
		) {
			return;
		}
		focusAfterRetry.current = false;
		if (query.data && document.activeElement === document.body) {
			triggerRef.current?.focus();
		}
	}, [query.isFetching, query.data, query.errorUpdatedAt]);

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
							disabled={!query.data}
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
								ref={triggerRef}
								id={selectId}
								className="max-w-xs text-content-primary"
								aria-disabled={mutation.isPending || undefined}
								aria-describedby={
									agentsEnabled
										? descriptionId
										: `${descriptionId} ${availabilityId}`
								}
							>
								<SelectValue
									placeholder={
										query.isFetching ? "Loading..." : "Select a page"
									}
								/>
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="workspaces">Workspaces</SelectItem>
								<SelectItem value="agents" disabled={!agentsEnabled}>
									Agents
								</SelectItem>
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
					{!agentsEnabled && (
						<p
							id={availabilityId}
							className="m-0 text-sm text-content-secondary"
						>
							Agents is unavailable. If it is your default, Workspaces opens
							instead.
						</p>
					)}
				</div>
				{Boolean(error) && <ErrorAlert error={error} />}
				{(query.isError || (query.isFetched && !query.data)) && (
					<Button
						variant="outline"
						className="w-fit"
						aria-disabled={query.isFetching || undefined}
						onClick={() => {
							if (!query.isFetching) {
								focusAfterRetry.current = true;
								void query.refetch();
							}
						}}
					>
						Retry loading preference
					</Button>
				)}
			</div>
		</Section>
	);
};
