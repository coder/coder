export const PERSONAL_SKILL_MAX_SIZE_BYTES = 64 * 1024;
const PERSONAL_SKILL_MAX_NAME_BYTES = 256;
export const PERSONAL_SKILLS_MAX_PER_USER = 100;

const personalSkillNamePattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const textEncoder = new TextEncoder();

export type PersonalSkillFormValues = {
	name: string;
	description: string;
	body: string;
};

class PersonalSkillMarkdownError extends Error {}

export const parsePersonalSkillMarkdown = (
	content: string,
): PersonalSkillFormValues => {
	const lines = content.replace(/^\uFEFF/, "").split("\n");
	if (lines[0]?.trim() !== "---") {
		throw new PersonalSkillMarkdownError(
			"Missing opening frontmatter delimiter.",
		);
	}

	const closingIndex = lines.findIndex(
		(line, index) => index > 0 && line.trim() === "---",
	);
	if (closingIndex < 0) {
		throw new PersonalSkillMarkdownError(
			"Missing closing frontmatter delimiter.",
		);
	}

	let name = "";
	let description = "";
	for (const line of lines.slice(1, closingIndex)) {
		const separatorIndex = line.indexOf(":");
		if (separatorIndex < 0) {
			continue;
		}
		const key = line.slice(0, separatorIndex).trim().toLowerCase();
		let value = line.slice(separatorIndex + 1).trim();
		if (value.length >= 2) {
			const first = value[0];
			const last = value[value.length - 1];
			if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
				value = value.slice(1, -1);
			}
		}
		if (key === "name") {
			name = value;
		} else if (key === "description") {
			description = value;
		}
	}

	const body = lines
		.slice(closingIndex + 1)
		.join("\n")
		.trim();

	if (!name) {
		throw new PersonalSkillMarkdownError("Skill name is required.");
	}
	if (!body) {
		throw new PersonalSkillMarkdownError("Skill body is required.");
	}

	return { name, description, body };
};

export const tryParsePersonalSkillMarkdown = (
	content: string,
):
	| { ok: true; values: PersonalSkillFormValues }
	| { ok: false; error: string } => {
	try {
		return { ok: true, values: parsePersonalSkillMarkdown(content) };
	} catch (error) {
		return {
			ok: false,
			error:
				error instanceof Error ? error.message : "Unable to parse SKILL.md.",
		};
	}
};

const frontmatterLineValue = (value: string): string =>
	value.replace(/\r?\n/g, " ").trim();

const frontmatterStringValue = (value: string): string =>
	`"${frontmatterLineValue(value)}"`;

export const buildPersonalSkillMarkdown = (
	values: PersonalSkillFormValues,
): string => {
	const name = frontmatterLineValue(values.name);
	const description = frontmatterLineValue(values.description);
	const body = values.body.trim();
	const frontmatter = ["---", `name: ${name}`];
	if (description) {
		frontmatter.push(`description: ${frontmatterStringValue(description)}`);
	}
	frontmatter.push("---");

	return `${frontmatter.join("\n")}\n${body}\n`;
};

export const getPersonalSkillContentSizeBytes = (content: string): number =>
	textEncoder.encode(content).length;

export const isValidPersonalSkillName = (name: string): boolean =>
	personalSkillNamePattern.test(name) &&
	getPersonalSkillContentSizeBytes(name) <= PERSONAL_SKILL_MAX_NAME_BYTES;
