import { useState } from "react";
import type {
	TemplateVersionVariable,
	VariableValue,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FormFields, VerticalForm } from "#/components/Form/Form";
import { Loader } from "#/components/Loader/Loader";
import { VariableInput } from "#/pages/CreateTemplatePage/VariableInput";

type MissingTemplateVariablesDialogProps = {
	open: boolean;
	onClose: () => void;
	onSubmit: (values: VariableValue[]) => void;
	missingVariables?: TemplateVersionVariable[];
};

export const MissingTemplateVariablesDialog: React.FC<
	MissingTemplateVariablesDialogProps
> = ({ missingVariables, onSubmit, open, onClose }) => {
	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onClose();
				}
			}}
		>
			<DialogContent className="max-w-md" data-testid="dialog">
				<DialogHeader>
					<DialogTitle>Template variables</DialogTitle>
					<DialogDescription>
						There are a few missing template variable values. Please fill them
						in.
					</DialogDescription>
				</DialogHeader>

				{missingVariables ? (
					<MissingVariablesForm
						missingVariables={missingVariables}
						onSubmit={onSubmit}
					/>
				) : (
					<Loader />
				)}

				<DialogFooter>
					<Button variant="outline" type="button" onClick={onClose}>
						Cancel
					</Button>
					<Button type="submit" form="updateVariables">
						Submit
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

type MissingVariablesFormProps = {
	missingVariables: readonly TemplateVersionVariable[];
	onSubmit: (values: VariableValue[]) => void;
};

const MissingVariablesForm: React.FC<MissingVariablesFormProps> = ({
	missingVariables,
	onSubmit,
}) => {
	const [editedValues, setEditedValues] = useState<Record<string, string>>({});

	return (
		<VerticalForm
			id="updateVariables"
			onSubmit={(e) => {
				e.preventDefault();
				onSubmit(
					missingVariables.map((v) => ({
						name: v.name,
						value: editedValues[v.name] ?? v.value,
					})),
				);
			}}
		>
			<FormFields>
				{missingVariables.map((variable) => {
					return (
						<VariableInput
							defaultValue={variable.value}
							variable={variable}
							key={variable.name}
							onChange={async (value) => {
								setEditedValues((prev) => ({
									...prev,
									[variable.name]: value,
								}));
							}}
						/>
					);
				})}
			</FormFields>
		</VerticalForm>
	);
};
