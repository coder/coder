import * as monaco from "monaco-editor";

export const registerHCLCompletionProvider = (
  monacoInstance: typeof monaco,
) => {
  monacoInstance.languages.registerCompletionItemProvider("hcl", {
    provideCompletionItems: function (model, position) {
      const suggestions = [
        {
          label: "resource",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define a new resource",
          insertText: 'resource "${1:type}" "${2:name}" {\n\t$0\n}',
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
        // Suggest data source
        {
          label: "data",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define a new data source",
          insertText: 'data "${1:type}" "${2:name}" {\n\t$0\n}',
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
        // Suggest locals
        {
          label: "locals",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define local variables",
          insertText: "locals {\n\t$0\n}",
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
        // Suggest variables
        {
          label: "variable",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define a new variable",
          insertText: 'variable "${1:name}" {\n\t$0\n}',
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
        // Suggest output
        {
          label: "output",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define a new output",
          insertText: 'output "${1:name}" {\n\t$0\n}',
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
        // Suggest module
        {
          label: "module",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define a new module",
          insertText: 'module "${1:name}" {\n\t$0\n}',
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
        // Suggest terraform
        {
          label: "terraform",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define a new terraform block",
          insertText: "terraform {\n\t$0\n}",
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
        // Suggest provider
        {
          label: "provider",
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          documentation: "Define a new provider",
          insertText: 'provider "${1:name}" {\n\t$0\n}',
          insertTextRules:
            monacoInstance.languages.CompletionItemInsertTextRule
              .InsertAsSnippet,
          range: {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: model.getWordUntilPosition(position).startColumn,
            endColumn: model.getWordUntilPosition(position).endColumn,
          },
        },
      ];

      return { suggestions: suggestions };
    },
  });
};
