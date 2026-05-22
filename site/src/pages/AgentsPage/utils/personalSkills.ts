import type * as TypesGen from "#/api/typesGenerated";

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

type RankedPersonalSkill = {
	skill: TypesGen.UserSkillMetadata;
	rank: number;
	index: number;
};

export const personalSkillTriggerText = (
	skill: TypesGen.UserSkillMetadata,
): string => `/${skill.name}`;

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
 * Filters personal skills by name and description. Matches are ranked by
 * name prefix, name substring, then description substring.
 */
export const filterPersonalSkills = (
	skills: readonly TypesGen.UserSkillMetadata[],
	query: string,
): TypesGen.UserSkillMetadata[] => {
	const normalizedQuery = query.toLocaleLowerCase("en-US");
	if (!normalizedQuery) {
		return skills.toSorted((a, b) => a.name.localeCompare(b.name, "en-US"));
	}

	return skills
		.map((skill, index): RankedPersonalSkill => {
			const name = skill.name.toLocaleLowerCase("en-US");
			const description = skill.description.toLocaleLowerCase("en-US");
			let rank = Number.POSITIVE_INFINITY;
			if (name.startsWith(normalizedQuery)) {
				rank = 0;
			} else if (name.includes(normalizedQuery)) {
				rank = 1;
			} else if (description.includes(normalizedQuery)) {
				rank = 2;
			}

			return { skill, rank, index };
		})
		.filter(({ rank }) => Number.isFinite(rank))
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

const unquoteFrontmatterScalar = (value: string): string => {
	if (value.length < 2) {
		return value;
	}

	const first = value[0];
	const last = value[value.length - 1];
	if (first !== last) {
		return value;
	}

	const inner = value.slice(1, -1);
	if (first === '"') {
		return inner.replaceAll('\\"', '"').replaceAll("\\\\", "\\");
	}
	if (first === "'") {
		return inner;
	}
	return value;
};

// This parser is only for projecting SKILL.md content into form fields.
// The API reparses and validates saved content on submit, so this mirrors the
// backend scalar subset instead of accepting full YAML semantics.
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
		const value = unquoteFrontmatterScalar(
			line.slice(separatorIndex + 1).trim(),
		);
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
	`"${frontmatterLineValue(value).replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;

export const isValidPersonalSkillDescription = (description: string): boolean =>
	getPersonalSkillContentSizeBytes(description) <=
	PERSONAL_SKILL_MAX_DESCRIPTION_BYTES;

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
