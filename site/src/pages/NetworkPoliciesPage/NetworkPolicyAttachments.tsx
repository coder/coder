import { useFormik } from "formik";
import { useRef, useState } from "react";
import type {
	NetworkPolicy,
	SetNetworkPolicyTemplatesRequest,
	Template,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { NetworkPolicyTemplateSelect } from "./NetworkPolicyFields";
import { isNetworkPolicyTargetBlocked } from "./networkPolicyTargets";

type NetworkPolicyAttachmentsProps = {
	policy: NetworkPolicy;
	templates: readonly Template[];
	isPending: boolean;
	error: unknown;
	onSubmit: (
		request: SetNetworkPolicyTemplatesRequest,
		onSuccess: (policy: NetworkPolicy) => void,
	) => void;
};

/** Attachment changes may immediately provision every target, including removals. */
export const NetworkPolicyAttachments: React.FC<
	NetworkPolicyAttachmentsProps
> = ({ policy, templates, isPending, error, onSubmit }) => {
	const [confirmation, setConfirmation] =
		useState<SetNetworkPolicyTemplatesRequest>();
	const hasBlockedTargets = policy.targets.some(isNetworkPolicyTargetBlocked);
	const saveRef = useRef<HTMLButtonElement>(null);
	const sectionRef = useRef<HTMLElement>(null);
	const form = useFormik<SetNetworkPolicyTemplatesRequest>({
		initialValues: {
			template_ids: policy.template_ids,
			expected_generation: policy.generation,
		},
		onSubmit: setConfirmation,
	});
	return (
		<section
			ref={sectionRef}
			tabIndex={-1}
			className="space-y-4"
			aria-label="Template attachments"
		>
			<h2 className="text-xl font-semibold">Template attachments</h2>
			{Boolean(error) && <ErrorAlert error={error} />}
			<form className="space-y-4" onSubmit={form.handleSubmit}>
				<NetworkPolicyTemplateSelect
					templates={templates}
					value={form.dirty ? form.values.template_ids : policy.template_ids}
					disabled={isPending}
					onChange={(template_ids) =>
						void form.setValues({
							template_ids,
							expected_generation: form.dirty
								? form.values.expected_generation
								: policy.generation,
						})
					}
				/>
				<p className="text-sm text-content-secondary">
					Saving attachments on a published policy immediately queues updates
					for all targets. Removing the last template removes the managed policy
					resources. Unpublished policies save attachments without provisioning.
				</p>
				{hasBlockedTargets && (
					<p className="text-sm text-content-destructive">
						Attachments cannot be updated while a target has an active operation
						or an unknown operation outcome. Unknown outcomes require operator
						investigation and reconciliation.
					</p>
				)}
				<div className="flex flex-wrap gap-2">
					<Button
						ref={saveRef}
						type="submit"
						disabled={!form.dirty || isPending || hasBlockedTargets}
					>
						Save attachments
					</Button>
					{form.dirty && (
						<Button
							type="button"
							variant="outline"
							disabled={isPending}
							onClick={() => {
								form.resetForm({
									values: {
										template_ids: policy.template_ids,
										expected_generation: policy.generation,
									},
								});
								sectionRef.current?.focus();
							}}
						>
							Discard attachment changes
						</Button>
					)}
				</div>
			</form>
			<ConfirmDialog
				open={Boolean(confirmation)}
				title="Update template attachments?"
				hideCancel={false}
				confirmText="Update attachments"
				confirmLoading={isPending}
				disabled={hasBlockedTargets}
				onClose={() => {
					if (!isPending) setConfirmation(undefined);
				}}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (saveRef.current?.disabled) sectionRef.current?.focus();
					else saveRef.current?.focus();
				}}
				onConfirm={() => {
					if (!confirmation) return;
					onSubmit(confirmation, (saved) => {
						form.resetForm({
							values: {
								template_ids: saved.template_ids,
								expected_generation: saved.generation,
							},
						});
						setConfirmation(undefined);
					});
				}}
				description={
					<div className="space-y-2">
						<p>
							{confirmation?.template_ids.length ?? 0} templates will be
							attached. On published policies this immediately queues updates on
							every target and can change workspace connectivity.
						</p>
						{confirmation?.template_ids.length === 0 && (
							<p>All managed network policy resources will be removed.</p>
						)}
						<ul>
							{policy.targets.map((target) => (
								<li key={target.id}>
									{target.cluster} / {target.context} / {target.namespace}
								</li>
							))}
						</ul>
						{hasBlockedTargets && (
							<p>
								A target is now busy or has an unknown outcome. Close this
								dialog and review target status before proceeding.
							</p>
						)}
						{Boolean(error) && <ErrorAlert error={error} />}
					</div>
				}
			/>
		</section>
	);
};
