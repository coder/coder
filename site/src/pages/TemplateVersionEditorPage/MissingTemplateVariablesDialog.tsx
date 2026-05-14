import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import { type FC, useEffect, useState } from "react";
import type {
	TemplateVersionVariable,
	VariableValue,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import type { DialogProps } from "#/components/Dialogs/Dialog";
import { FormFields, VerticalForm } from "#/components/Form/Form";
import { Loader } from "#/components/Loader/Loader";
import { VariableInput } from "#/pages/CreateTemplatePage/VariableInput";

type MissingTemplateVariablesDialogProps = Omit<DialogProps, "onSubmit"> & {
	onClose: () => void;
	onSubmit: (values: VariableValue[]) => void;
	missingVariables?: TemplateVersionVariable[];
};

export const MissingTemplateVariablesDialog: FC<
	MissingTemplateVariablesDialogProps
> = ({ missingVariables, onSubmit, ...dialogProps }) => {
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
			{...dialogProps}
			scroll="body"
			aria-labelledby="update-build-parameters-title"
			maxWidth="xs"
			data-testid="dialog"
		>
			<DialogTitle
				id="update-build-parameters-title"
				className="px-10 py-6 text-xl font-normal"
			>
				Template variables
			</DialogTitle>
			<DialogContent className="px-10">
				<DialogContentText className="m-0">
					There are a few missing template variable values. Please fill them in.
				</DialogContentText>
				<VerticalForm
					className="pt-8"
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
											setVariableValues((prev) => {
												prev[index] = {
													name: variable.name,
													value,
												};
												return [...prev];
											});
										}}
									/>
								);
							})}
						</FormFields>
					) : (
						<Loader />
					)}
				</VerticalForm>
			</DialogContent>
			<DialogActions disableSpacing className="flex flex-col gap-2 p-10">
				<Button className="w-full" type="submit" form="updateVariables">
					Submit
				</Button>
				<Button
					variant="outline"
					className="w-full"
					type="button"
					onClick={dialogProps.onClose}
				>
					Cancel
				</Button>
			</DialogActions>
		</Dialog>
	);
};
