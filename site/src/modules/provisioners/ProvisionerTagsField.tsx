import TextField from "@mui/material/TextField";
import type { ProvisionerDaemon } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { PlusIcon } from "lucide-react";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTag";
import { type FC, useRef, useState } from "react";
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

			<NewTagControl
				onAdd={(tag) => {
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

type Tag = { key: string; value: string };

type NewTagControlProps = {
	onAdd: (tag: Tag) => void;
};

const NewTagControl: FC<NewTagControlProps> = ({ onAdd }) => {
	const keyInputRef = useRef<HTMLInputElement>(null);
	const [error, setError] = useState<string>();
	const [newTag, setNewTag] = useState<Tag>({
		key: "",
		value: "",
	});

	const addNewTag = async () => {
		try {
			await newTagSchema.validate(newTag);
			onAdd(newTag);
			setNewTag({ key: "", value: "" });
			keyInputRef.current?.focus();
		} catch (e) {
			const isValidationError = e instanceof Yup.ValidationError;

			if (!isValidationError) {
				throw e;
			}

			if (e instanceof Yup.ValidationError) {
				setError(e.errors[0]);
			}
		}
	};

	const addNewTagOnEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
		if (e.key === "Enter") {
			e.preventDefault();
			e.stopPropagation();
			addNewTag();
		}
	};

	return (
		<div className="flex flex-col gap-1 max-w-72">
			<div className="flex items-center gap-2">
				<label className="sr-only" htmlFor="tag-key-input">
					Tag key
				</label>
				<TextField
					inputRef={keyInputRef}
					size="small"
					id="tag-key-input"
					name="key"
					placeholder="Key"
					value={newTag.key}
					onChange={(e) => setNewTag({ ...newTag, key: e.target.value.trim() })}
					onKeyDown={addNewTagOnEnter}
				/>

				<label className="sr-only" htmlFor="tag-value-input">
					Tag value
				</label>
				<TextField
					size="small"
					id="tag-value-input"
					name="value"
					placeholder="Value"
					value={newTag.value}
					onChange={(e) =>
						setNewTag({ ...newTag, value: e.target.value.trim() })
					}
					onKeyDown={addNewTagOnEnter}
				/>

				<Button
					className="flex-shrink-0"
					size="icon"
					type="button"
					onClick={addNewTag}
				>
					<PlusIcon />
					<span className="sr-only">Add tag</span>
				</Button>
			</div>
			{error && (
				<span className="text-xs text-content-destructive">{error}</span>
			)}
		</div>
	);
};
