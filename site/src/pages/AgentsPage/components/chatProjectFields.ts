import * as Yup from "yup";

// Keep in sync with chatProject*MaxChars in coderd/chat_projects.go.
export const chatProjectNameMaxChars = 64;
export const chatProjectDescriptionMaxChars = 1024;
export const chatProjectIconMaxChars = 256;

// Counts code points of the value as submitted, as the server does. Yup's
// max() and native maxLength count UTF-16 units.
export const measureLength = (value: string) => [...value.trim()].length;

const tooLong = (label: string, max: number) => (value: string) =>
	measureLength(value) > max
		? `${label} cannot be longer than ${max} characters.`
		: undefined;

/** Returns the message for a name that cannot be saved. */
export const validateChatProjectName = (value: string) =>
	value.trim() === ""
		? "Name is required."
		: tooLong("Name", chatProjectNameMaxChars)(value);

export const validateChatProjectDescription = tooLong(
	"Description",
	chatProjectDescriptionMaxChars,
);

export const validateChatProjectIcon = tooLong("Icon", chatProjectIconMaxChars);

const fieldSchema = (validate: (value: string) => string | undefined) =>
	Yup.string()
		.trim()
		.test("valid", function (value = "") {
			const message = validate(value);
			return message ? this.createError({ message }) : true;
		});

export const chatProjectFieldsSchema = Yup.object({
	name: fieldSchema(validateChatProjectName),
	description: fieldSchema(validateChatProjectDescription),
	icon: fieldSchema(validateChatProjectIcon),
});
