import { RuleTester } from "oxlint/plugins-dev";
import { describe, it } from "vitest";
import plugin from "./oxlint-plugin-coder.mjs";

RuleTester.describe = describe;
RuleTester.it = it;

const ruleTester = new RuleTester({
	languageOptions: { parserOptions: { lang: "tsx" } },
});

const error = (name) => ({ messageId: "preferNamespace", data: { name } });

ruleTester.run(
	"prefer-react-namespace-types",
	plugin.rules["prefer-react-namespace-types"],
	{
		valid: [
			'import { useState } from "react";',
			'import type React from "react";',
			'import * as React from "react";',
			'import type { Foo } from "./react";',
			"const C: React.FC = () => null;",
		],
		invalid: [
			{
				code: 'import type { FC, ReactNode } from "react";\nconst C: FC<{ c: ReactNode }> = () => null;\n',
				output: "const C: React.FC<{ c: React.ReactNode }> = () => null;\n",
				errors: [error("FC"), error("ReactNode")],
			},
			{
				code: 'import { type Ref, useState } from "react";\nlet r: Ref<HTMLDivElement>;\n',
				output:
					'import { useState } from "react";\nlet r: React.Ref<HTMLDivElement>;\n',
				errors: [error("Ref")],
			},
			{
				code: "import {\n\tuseState,\n\ttype SubmitEvent,\n} from 'react';\nlet e: SubmitEvent;\n",
				output: 'import { useState } from "react";\nlet e: React.SubmitEvent;\n',
				errors: [error("SubmitEvent")],
			},
			{
				code: 'import React, { type FC } from "react";\nconst C: FC = () => React.useId();\n',
				output: 'import React from "react";\nconst C: React.FC = () => React.useId();\n',
				errors: [error("FC")],
			},
			{
				code: 'import type { KeyboardEvent as KE, JSX } from "react";\nlet e: KE;\nlet j: JSX.Element;\n',
				output: "let e: React.KeyboardEvent;\nlet j: React.JSX.Element;\n",
				errors: [error("KeyboardEvent"), error("JSX")],
			},
		],
	},
);
