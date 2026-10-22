import { useFormik } from "formik";
import { useId, useRef } from "react";
import type {
	CreateNetworkPolicyRevisionRequest,
	NetworkPolicy,
	NetworkPolicyRevision,
} from "#/api/typesGenerated";
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
import { getFormHelpers } from "#/utils/formUtils";
import { NetworkPolicyHCL } from "./NetworkPolicyFields";

type NetworkPolicyEditorProps = {
	policy: NetworkPolicy;
	selectedRevisionId: string;
	onSelectRevision: (id: string) => void;
	isPending: boolean;
	readOnly: boolean;
	error: unknown;
	onSave: (
		request: CreateNetworkPolicyRevisionRequest,
		onSuccess: (revision: NetworkPolicyRevision) => void,
	) => void;
};

/** Retains unsaved HCL and its concurrency token through background refreshes. */
export const NetworkPolicyEditor: React.FC<NetworkPolicyEditorProps> = ({
	policy,
	selectedRevisionId,
	onSelectRevision,
	isPending,
	readOnly,
	error,
	onSave,
}) => {
	const id = useId();
	const sectionRef = useRef<HTMLElement>(null);
	const revision = policy.revisions.find(
		(revision) => revision.id === selectedRevisionId,
	);
	const form = useFormik<CreateNetworkPolicyRevisionRequest>({
		initialValues: {
			hcl: revision?.hcl ?? "",
			expected_generation: policy.generation,
		},
		onSubmit: (request) => {
			if (isPending || readOnly) return;
			onSave(request, (saved) => {
				form.resetForm({
					values: { hcl: saved.hcl, expected_generation: policy.generation },
				});
				onSelectRevision(saved.id);
			});
		},
	});
	const getField = getFormHelpers(form, error);
	return (
		<section
			ref={sectionRef}
			tabIndex={-1}
			className="space-y-4"
			aria-label="Policy versions"
		>
			<h2 className="text-xl font-semibold">Versions</h2>
			{Boolean(error) && <ErrorAlert error={error} />}
			<div className="space-y-2">
				<Label htmlFor={id}>Version</Label>
				<Select
					value={selectedRevisionId}
					onValueChange={(id) => {
						const selected = policy.revisions.find(
							(revision) => revision.id === id,
						);
						if (!selected) return;
						form.resetForm({
							values: {
								hcl: selected.hcl,
								expected_generation: policy.generation,
							},
						});
						onSelectRevision(id);
					}}
					disabled={form.dirty || isPending}
				>
					<SelectTrigger id={id}>
						<SelectValue placeholder="Select a version" />
					</SelectTrigger>
					<SelectContent>
						{policy.revisions
							.toSorted((a, b) => a.created_at.localeCompare(b.created_at))
							.map((revision, index) => (
								<SelectItem key={revision.id} value={revision.id}>
									Version {index + 1} ({revision.id.slice(0, 8)})
								</SelectItem>
							))}
					</SelectContent>
				</Select>
			</div>
			{revision ? (
				<form className="space-y-4" onSubmit={form.handleSubmit}>
					<NetworkPolicyHCL
						field={getField("hcl")}
						disabled={isPending}
						readOnly={readOnly}
						onBlur={() => void form.setFieldTouched("hcl", true)}
						onChange={(hcl) =>
							void form.setValues({
								hcl,
								expected_generation: form.dirty
									? form.values.expected_generation
									: policy.generation,
							})
						}
					/>
					{form.dirty && (
						<p className="text-sm text-content-secondary">
							{readOnly
								? "This policy is read-only. Discard unsaved changes to inspect saved versions."
								: "Unsaved changes. Save a new version before publishing. Switching versions is disabled until you save or discard."}
						</p>
					)}
					<div className="flex gap-2">
						<Button
							type="submit"
							disabled={
								!form.dirty || !form.values.hcl.trim() || isPending || readOnly
							}
						>
							Save new version
						</Button>
						{form.dirty && (
							<Button
								type="button"
								variant="outline"
								disabled={isPending}
								onClick={() => {
									form.resetForm({
										values: {
											hcl: revision.hcl,
											expected_generation: policy.generation,
										},
									});
									sectionRef.current?.focus();
								}}
							>
								Discard edits
							</Button>
						)}
					</div>
				</form>
			) : (
				<p className="text-content-secondary">No version selected.</p>
			)}
		</section>
	);
};
