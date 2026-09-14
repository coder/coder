import frontMatter from "front-matter";

export const PERSONAL_SKILL_MAX_SIZE_BYTES = 64 * 1024;
const PERSONAL_SKILL_MAX_NAME_BYTES = 256;
const PERSONAL_SKILL_MAX_DESCRIPTION_BYTES = 4096;
export const PERSONAL_SKILLS_MAX_PER_USER = 100;

const personalSkillNamePattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const textEncoder = new TextEncoder();

export type PersonalSkillFormValues = {
	name: string;
	description: string;
	body: string;
};

type SkillSearchMetadata = {
	name: string;
	description: string;
	triggerText?: string;
	// Alternate trigger form that stays searchable regardless of which
	// form is displayed, e.g. the qualified /source/name alias.
	altTriggerText?: string;
};

type RankedSkill<T extends SkillSearchMetadata> = {
	skill: T;
	rank: number;
	index: number;
};

export const personalSkillTriggerText = (skill: { name: string }): string =>
	`/${skill.name}`;

type PersonalSkillTriggerMatch = {
	slashOffset: number;
	query: string;
};

export const parsePersonalSkillTrigger = (
	linePrefix: string,
): PersonalSkillTriggerMatch | null => {
	const match = /(?:^|\s)\/(\S*)$/.exec(linePrefix);
	if (!match) {
		return null;
	}

	return {
		slashOffset: match.index + match[0].indexOf("/"),
		query: match[1] ?? "",
	};
};

export const isPersonalSkillTriggerToken = (token: string): boolean =>
	/^\/\S*$/.test(token);

/**
 * Filters skills by name, trigger text, and description. Matches are ranked
 * by name or trigger text prefix, name or trigger text substring, then
 * description substring.
 */
export const filterSkillsByQuery = <T extends SkillSearchMetadata>(
	skills: readonly T[],
	query: string,
): T[] => {
	const normalizedQuery = query.toLocaleLowerCase("en-US");
	if (!normalizedQuery) {
		return skills.toSorted((a, b) => a.name.localeCompare(b.name, "en-US"));
	}

	const rankedSkills: RankedSkill<T>[] = [];
	for (const [index, skill] of skills.entries()) {
		const name = skill.name.toLocaleLowerCase("en-US");
		const triggerText = skill.triggerText
			?.replace(/^\//, "")
			.toLocaleLowerCase("en-US");
		const altTriggerText = skill.altTriggerText
			?.replace(/^\//, "")
			.toLocaleLowerCase("en-US");
		const description = skill.description.toLocaleLowerCase("en-US");
		let rank: number | undefined;
		if (
			name.startsWith(normalizedQuery) ||
			triggerText?.startsWith(normalizedQuery) ||
			altTriggerText?.startsWith(normalizedQuery)
		) {
			rank = 0;
		} else if (
			name.includes(normalizedQuery) ||
			triggerText?.includes(normalizedQuery) ||
			altTriggerText?.includes(normalizedQuery)
		) {
			rank = 1;
		} else if (description.includes(normalizedQuery)) {
			rank = 2;
		}
		if (rank !== undefined) {
			rankedSkills.push({ skill, rank, index });
		}
	}

	return rankedSkills
		.toSorted((a, b) => {
			if (a.rank !== b.rank) {
				return a.rank - b.rank;
			}
			const nameOrder = a.skill.name.localeCompare(b.skill.name, "en-US");
			return nameOrder === 0 ? a.index - b.index : nameOrder;
		})
		.map(({ skill }) => skill);
};

class PersonalSkillMarkdownError extends Error {}

const frontmatterStringField = (
	attributes: Record<string, unknown>,
	key: "name" | "description",
): string => {
	const value = attributes[key];
	if (value === undefined) {
		return "";
	}
	if (typeof value !== "string") {
		throw new PersonalSkillMarkdownError(`Skill ${key} must be a string.`);
	}
	return value.replace(/[\r\n]+$/, "");
};

// The API re-validates on submit; this only projects content into form fields.
export const parsePersonalSkillMarkdown = (
	content: string,
): PersonalSkillFormValues => {
	const normalizedContent = content.replace(/^\uFEFF/, "");
	const lines = normalizedContent.split("\n");
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

	const parseableContent = [
		"---",
		...lines.slice(1, closingIndex),
		"---",
		...lines.slice(closingIndex + 1),
	].join("\n");
	const parsed = (() => {
		try {
			return frontMatter<Record<string, unknown>>(parseableContent);
		} catch (error) {
			const message = error instanceof Error ? error.message : "unknown error";
			throw new PersonalSkillMarkdownError(`Invalid frontmatter: ${message}`);
		}
	})();

	const name = frontmatterStringField(parsed.attributes, "name");
	const description = frontmatterStringField(parsed.attributes, "description");
	const body = parsed.body.trim();

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
	`"${frontmatterLineValue(value).replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;

const frontmatterNameValue = (value: string): string => {
	const lineValue = frontmatterLineValue(value);
	if (/^(?:true|false|null)$/.test(lineValue) || /^[0-9]/.test(lineValue)) {
		return frontmatterStringValue(lineValue);
	}
	return lineValue;
};

export const isValidPersonalSkillDescription = (description: string): boolean =>
	getPersonalSkillContentSizeBytes(description) <=
	PERSONAL_SKILL_MAX_DESCRIPTION_BYTES;

export const buildPersonalSkillMarkdown = (
	values: PersonalSkillFormValues,
): string => {
	const name = frontmatterNameValue(values.name);
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
