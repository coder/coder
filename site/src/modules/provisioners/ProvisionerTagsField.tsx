import type { ProvisionerDaemon } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { PlusIcon } from "lucide-react";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTag";
import { type FC, useState } from "react";
import * as Yup from "yup";

// Users can't delete these tags
const REQUIRED_TAGS = ["scope", "organization", "user"];

// Users can't override these tags
const IMMUTABLE_TAGS = ["owner"];

type ProvisionerTagsFieldProps = {
	value: ProvisionerDaemon["tags"];
	onChange: (value: ProvisionerDaemon["tags"]) => void;
};

export const ProvisionerTagsField: FC<ProvisionerTagsFieldProps> = ({
	value: fieldValue,
	onChange,
}) => {
	return (
		<div className="flex flex-col gap-3">
			<div className="flex items-center gap-2 flex-wrap">
				{Object.entries(fieldValue)
					// Filter out since users cannot override it
					.filter(([key]) => !IMMUTABLE_TAGS.includes(key))
					.map(([key, value]) => {
						const onDelete = (key: string) => {
							const { [key]: _, ...newFieldValue } = fieldValue;
							onChange(newFieldValue);
						};

						return (
							<ProvisionerTag
								key={key}
								tagName={key}
								tagValue={value}
								// Required tags can't be deleted
								onDelete={REQUIRED_TAGS.includes(key) ? undefined : onDelete}
							/>
						);
					})}
			</div>

			<NewTagForm
				onSubmit={(tag) => {
					onChange({ ...fieldValue, [tag.key]: tag.value });
				}}
			/>
		</div>
	);
};

const newTagSchema = Yup.object({
	key: Yup.string()
		.required("Key is required")
		.notOneOf(["owner"], "Cannot override owner tag"),
	value: Yup.string()
		.required("Value is required")
		.when("key", ([key], schema) => {
			if (key === "scope") {
				return schema.oneOf(
					["organization", "scope"],
					"Scope value must be 'organization' or 'user'",
				);
			}

			return schema;
		}),
});

type NewTagFormProps = {
	onSubmit: (values: { key: string; value: string }) => void;
};

const NewTagForm: FC<NewTagFormProps> = ({ onSubmit }) => {
	const [error, setError] = useState<string>();

	return (
		<form
			className="flex flex-col gap-1"
			onSubmit={async (e) => {
				e.preventDefault();
				const form = e.currentTarget;
				const key = form.key.value.trim();
				const value = form.value.value.trim();

				try {
					await newTagSchema.validate({ key, value });
					onSubmit({ key, value });
					form.reset();
				} catch (e) {
					const isValidationError = e instanceof Yup.ValidationError;

					if (!isValidationError) {
						throw e;
					}

					if (e instanceof Yup.ValidationError) {
						setError(e.errors[0]);
					}
				}
			}}
		>
			<div className="flex items-center gap-2">
				<label className="sr-only" htmlFor="tag-key-input">
					Tag key
				</label>
				<Input
					id="tag-key-input"
					name="key"
					placeholder="Key"
					className="h-8 md:text-xs px-2"
					required
				/>

				<label className="sr-only" htmlFor="tag-value-input">
					Tag value
				</label>
				<Input
					id="tag-value-input"
					name="value"
					placeholder="Value"
					className="h-8 md:text-xs px-2"
					required
				/>

				<Button size="icon" type="submit">
					<PlusIcon />
					<span className="sr-only">Add tag</span>
				</Button>
			</div>
			{error && (
				<span className="text-xs text-content-destructive">{error}</span>
			)}
		</form>
	);
};
