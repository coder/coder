import { RuleTester } from "oxlint/plugins-dev";
import { describe, it } from "vitest";
import plugin from "./oxlint-plugin-coder.mjs";

RuleTester.describe = describe;
RuleTester.it = it;

const ruleTester = new RuleTester({
	languageOptions: { parserOptions: { lang: "tsx" } },
});

const preferNamespace = (name) => ({
	messageId: "preferNamespace",
	data: { name },
});

ruleTester.run(
	"prefer-react-namespace-types",
	plugin.rules["prefer-react-namespace-types"],
	{
		valid: [
			{
				name: "value imports from react",
				code: `import { useState } from "react";`,
			},
			{
				name: "default type import of React",
				code: `import type React from "react";`,
			},
			{
				name: "namespace import of React",
				code: `import * as React from "react";`,
			},
			{
				name: "type imports from other modules",
				code: `import type { Foo } from "./react";`,
			},
			{
				name: "React namespace without an import",
				code: `const Foo: React.FC = () => null;`,
			},
		],
		invalid: [
			{
				name: "import type declaration is removed",
				code: `
import type { FC, ReactNode } from "react";
const Foo: FC<{ children: ReactNode }> = () => null;
`,
				output: `
const Foo: React.FC<{ children: React.ReactNode }> = () => null;
`,
				errors: [preferNamespace("FC"), preferNamespace("ReactNode")],
			},
			{
				name: "inline type specifier is removed, value imports are kept",
				code: `
import { type Ref, useState } from "react";
let ref: Ref<HTMLDivElement>;
`,
				output: `
import { useState } from "react";
let ref: React.Ref<HTMLDivElement>;
`,
				errors: [preferNamespace("Ref")],
			},
			{
				name: "multiline import with single quotes",
				code: `
import {
	useState,
	type SubmitEvent,
} from 'react';
let event: SubmitEvent;
`,
				output: `
import { useState } from "react";
let event: React.SubmitEvent;
`,
				errors: [preferNamespace("SubmitEvent")],
			},
			{
				name: "default import is kept",
				code: `
import React, { type FC } from "react";
const Foo: FC = () => React.useId();
`,
				output: `
import React from "react";
const Foo: React.FC = () => React.useId();
`,
				errors: [preferNamespace("FC")],
			},
			{
				name: "aliased and namespace types use the exported name",
				code: `
import type { KeyboardEvent as KE, JSX } from "react";
let event: KE;
let element: JSX.Element;
`,
				output: `
let event: React.KeyboardEvent;
let element: React.JSX.Element;
`,
				errors: [preferNamespace("KeyboardEvent"), preferNamespace("JSX")],
			},
			{
				name: "identifiers that are not references are untouched",
				code: `
import type { FC } from "react";
const Foo: FC = () => null;
const props = { FC: 1 };
`,
				output: `
const Foo: React.FC = () => null;
const props = { FC: 1 };
`,
				errors: [preferNamespace("FC")],
			},
		],
	},
);
