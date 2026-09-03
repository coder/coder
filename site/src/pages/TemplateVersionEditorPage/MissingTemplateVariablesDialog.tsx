import { type FC, useEffect, useState } from "react";
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

export const MissingTemplateVariablesDialog: FC<
	MissingTemplateVariablesDialogProps
> = ({ missingVariables, onSubmit, open, onClose }) => {
	const [variableValues, setVariableValues] = useState<VariableValue[]>([]);

	// Pre-fill the form with the default values when missing variables are loaded
	useEffect(() => {
		if (!missingVariables) {
			return;
		}
		setVariableValues(
			missingVariables.map((v) => ({ name: v.name, value: v.value })),
		);
	}, [missingVariables]);

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

				<VerticalForm
					id="updateVariables"
					onSubmit={(e) => {
						e.preventDefault();
						onSubmit(variableValues);
					}}
				>
					{missingVariables ? (
						<FormFields>
							{missingVariables.map((variable, index) => {
								return (
									<VariableInput
										defaultValue={variable.value}
										variable={variable}
										key={variable.name}
										onChange={async (value) => {
											setVariableValues((prev) =>
												prev.with(index, { name: variable.name, value }),
											);
										}}
									/>
								);
							})}
						</FormFields>
					) : (
						<Loader />
					)}
				</VerticalForm>

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
