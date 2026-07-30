import { defineConfig } from "oxlint";

import confusingBrowserGlobals from "confusing-browser-globals";

export default defineConfig({
  $schema: "./node_modules/oxlint/configuration_schema.json",
  plugins: ["react", "jsx-a11y"],
  categories: {
    correctness: "error",
  },
  env: {
    builtin: true,
  },
  ignorePatterns: ["dist/**/*"],
  rules: {
    "no-case-declarations": "error",
    "no-empty": "error",
    "no-fallthrough": "error",
    "no-prototype-builtins": "error",
    "no-redeclare": "error",
    "no-regex-spaces": "error",
    "no-unexpected-multiline": "error",
    "react/display-name": "error",
    "react/jsx-no-comment-textnodes": "error",
    "react/jsx-no-target-blank": "error",
    "react/no-unescaped-entities": "error",
    "react/no-unknown-property": "error",
    "no-restricted-globals": ["error", ...confusingBrowserGlobals],
    "react/rules-of-hooks": "error",
  },
  overrides: [
    {
      files: ["**/*.ts", "**/*.tsx", "**/*.mts", "**/*.cts"],
      rules: {
        "no-redeclare": "off",
        "no-var": "error",
        "prefer-const": "error",
        "prefer-rest-params": "error",
        "prefer-spread": "error",
      },
    },
    {
      files: ["**/*.ts?(x)"],
      rules: {
        "no-array-constructor": "error",
        "typescript/ban-ts-comment": "error",
        "typescript/no-empty-object-type": "error",
        "typescript/no-explicit-any": "error",
        "typescript/no-require-imports": "error",
        "typescript/no-unnecessary-type-constraint": "error",
      },
      plugins: ["typescript"],
    },
  ],
});