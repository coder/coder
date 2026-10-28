import { useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link } from "react-router";
import { foundations, setTemplateFoundation } from "#/api/queries/foundations";
import type { Template } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

type FoundationBindingProps = {
	template: Template;
	canCreate: boolean;
	canBind: boolean;
};

/** Attaches an existing Foundation without starting a Terraform operation. */
export const FoundationBinding: React.FC<FoundationBindingProps> = ({
	template,
	canCreate,
	canBind,
}) => {
	const client = useQueryClient();
	const id = useId();
	const [selected, setSelected] = useState("");
	const [confirm, setConfirm] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);
	const section = useRef<HTMLElement>(null);
	const list = useQuery(foundations(template.organization_id));
	const bind = useMutation(
		setTemplateFoundation(client, template.organization_id, template.id),
	);
	const available = list.data?.filter((item) => !item.template_id);
	return (
		<section
			ref={section}
			tabIndex={-1}
			className="space-y-4 p-6"
			aria-label="Configure Foundation"
		>
			<h2 className="text-xl font-semibold">No Foundation attached</h2>
			<p className="text-content-secondary">
				A Foundation owns shared Terraform independently of workspace versions.
				Attaching one does not run Terraform or rebuild workspaces. Before the
				first Apply, Foundation outputs are empty. Workspace configurations that
				require those outputs cannot successfully plan.
			</p>
			{!canBind && (
				<p>
					Template update and Foundation use permission are required to attach a
					Foundation.
				</p>
			)}
			{canCreate ? (
				<Button asChild>
					<Link
						to={`/foundations/new?organization=${encodeURIComponent(template.organization_id)}`}
					>
						Create Foundation
					</Link>
				</Button>
			) : (
				<p>Foundation create permission is required to create one.</p>
			)}
			{Boolean(bind.error) && <ErrorAlert error={bind.error} />}
			<div className="space-y-2">
				<Label htmlFor={`${id}-existing`}>
					Available organization Foundations
				</Label>
				{Boolean(list.error) && (
					<>
						<ErrorAlert error={list.error} />
						<Button variant="outline" onClick={() => void list.refetch()}>
							Retry Foundations
						</Button>
					</>
				)}
				<Select
					value={selected}
					onValueChange={(value) => {
						setSelected(value);
						bind.reset();
					}}
					disabled={bind.isPending || !canBind || list.isLoading}
				>
					<SelectTrigger id={`${id}-existing`}>
						<SelectValue
							placeholder={
								list.isLoading
									? "Loading Foundations…"
									: "Choose an unattached Foundation"
							}
						/>
					</SelectTrigger>
					<SelectContent>
						{available?.map((item) => (
							<SelectItem key={item.id} value={item.id}>
								{item.name} ({item.id})
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				{available?.length === 0 && (
					<p>No unattached Foundations are available.</p>
				)}
				<Button
					ref={trigger}
					variant="outline"
					disabled={
						bind.isPending ||
						!canBind ||
						!available?.some((item) => item.id === selected)
					}
					onClick={() => {
						if (
							canBind &&
							!bind.isPending &&
							available?.some((item) => item.id === selected)
						) {
							setConfirm(true);
						}
					}}
				>
					Attach Foundation
				</Button>
			</div>
			<ConfirmDialog
				open={confirm}
				hideCancel={false}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (trigger.current?.disabled) section.current?.focus();
					else trigger.current?.focus();
				}}
				title="Attach Foundation?"
				confirmText={bind.isError ? "Retry attachment" : "Confirm attachment"}
				confirmLoading={bind.isPending}
				disabled={
					!canBind ||
					bind.isPending ||
					!available?.some((item) => item.id === selected)
				}
				onClose={() => {
					if (!bind.isPending) setConfirm(false);
				}}
				onConfirm={() => {
					if (
						canBind &&
						!bind.isPending &&
						available?.some((item) => item.id === selected)
					)
						bind.mutate(
							{ foundation_id: selected },
							{ onSuccess: () => setConfirm(false) },
						);
				}}
				description={
					<div className="space-y-3">
						<p>
							Attaching this Foundation delegates access to its outputs,
							including sensitive values, to this template and its editors.
							Template code can pass those values into workspaces. Only attach a
							Foundation to a template whose editors you trust.
						</p>
						<p>
							Attaching does not run Terraform or rebuild existing workspaces.
						</p>
						{Boolean(bind.error) && <ErrorAlert error={bind.error} />}
					</div>
				}
			/>
		</section>
	);
};
