import { type FC, useId } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import { Switch } from "#/components/Switch/Switch";

export const CollapseAssistantStepsSettings: FC = () => {
	const queryClient = useQueryClient();
	const query = useQuery(preferenceSettings());
	const mutation = useMutation(updatePreferenceSettings(queryClient));
	const descriptionId = useId();

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Collapse assistant steps
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p
					id={descriptionId}
					className="m-0 flex-1 text-xs text-content-secondary"
				>
					Collapse each turn's steps into a single row showing how long the
					agent worked. Answers and questions stay visible; failed steps are
					counted on the row.
				</p>
				<Switch
					checked={query.data?.collapse_assistant_steps ?? false}
					disabled={!query.data || mutation.isPending}
					onCheckedChange={(checked) => {
						mutation.mutate({ collapse_assistant_steps: checked });
					}}
					aria-label="Collapse assistant steps"
					aria-describedby={descriptionId}
				/>
			</div>
			{query.isError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load your collapse assistant steps preference.
				</p>
			)}
			{mutation.isError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save your collapse assistant steps preference.
				</p>
			)}
		</div>
	);
};
